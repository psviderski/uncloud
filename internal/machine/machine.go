package machine

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/sockets"
	"github.com/siderolabs/grpc-proxy/proxy"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"uncloud/internal/corrosion"
	"uncloud/internal/machine/api/pb"
	apiproxy "uncloud/internal/machine/api/proxy"
	"uncloud/internal/machine/cluster"
	"uncloud/internal/machine/corroservice"
	"uncloud/internal/machine/docker"
	"uncloud/internal/machine/network"
	"uncloud/internal/machine/store"
)

const (
	DefaultMachineSockPath  = "/run/uncloud/machine.sock"
	DefaultUncloudSockPath  = "/run/uncloud/uncloud.sock"
	DefaultUncloudSockGroup = "uncloud"
)

type Config struct {
	// DataDir is the directory where the machine stores its persistent state.
	DataDir         string
	MachineSockPath string
	UncloudSockPath string

	CorrosionDir           string
	CorrosionAPIListenAddr netip.AddrPort
	CorrosionAPIAddr       netip.AddrPort
	CorrosionAdminSockPath string
	CorrosionService       corroservice.Service
}

// SetDefaults returns a new Config with default values set where not provided.
func (c *Config) SetDefaults() *Config {
	// Copy c into a new Config to avoid modifying the original.
	cfg := *c

	if cfg.DataDir == "" {
		cfg.DataDir = "/var/lib/uncloud"
	}
	if cfg.MachineSockPath == "" {
		cfg.MachineSockPath = DefaultMachineSockPath
	}
	if cfg.UncloudSockPath == "" {
		cfg.UncloudSockPath = DefaultUncloudSockPath
	}
	if cfg.CorrosionDir == "" {
		cfg.CorrosionDir = filepath.Join(cfg.DataDir, "corrosion")
	}
	if !cfg.CorrosionAPIListenAddr.IsValid() {
		cfg.CorrosionAPIListenAddr = netip.AddrPortFrom(
			netip.AddrFrom4([4]byte{127, 0, 0, 1}), corroservice.DefaultAPIPort)
	}
	if !cfg.CorrosionAPIAddr.IsValid() {
		cfg.CorrosionAPIAddr = netip.AddrPortFrom(
			netip.AddrFrom4([4]byte{127, 0, 0, 1}), corroservice.DefaultAPIPort)
	}
	if cfg.CorrosionAdminSockPath == "" {
		cfg.CorrosionAdminSockPath = filepath.Join(cfg.CorrosionDir, "admin.sock")
	}
	if cfg.CorrosionService == nil {
		cfg.CorrosionService = corroservice.DefaultSystemdService(cfg.CorrosionDir)
	}
	return &cfg
}

type Machine struct {
	pb.UnimplementedMachineServer

	config Config
	state  *State
	// started is closed when the machine is ready to serve requests on the local API server.
	started chan struct{}
	// initialised is signalled when the machine is configured as a member of a cluster.
	initialised chan struct{}

	// store is the cluster store backed by a distributed Corrosion database.
	store   *store.Store
	cluster *cluster.Cluster
	docker  *docker.Server
	// localMachineServer is the gRPC server for the machine API listening on the local Unix socket.
	localMachineServer *grpc.Server

	// proxyDirector manages routing of gRPC requests between local and remote machine API servers.
	proxyDirector *apiproxy.Director
	// localProxyServer is the gRPC proxy server for the machine API listening on the local Unix socket.
	// It proxies requests to the local or remote machine API servers depending on the request targets
	// and aggregates responses.
	localProxyServer *grpc.Server
}

func NewMachine(config *Config) (*Machine, error) {
	config = config.SetDefaults()

	// Load the existing machine state or create a new one.
	statePath := StatePath(config.DataDir)
	state, err := ParseState(statePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load machine state: %w", err)
		}
		// Generate an empty machine config with a new key pair.
		slog.Info("Machine state file not found, creating a new one.", "path", statePath)
		privKey, pubKey, kErr := network.NewMachineKeys()
		if kErr != nil {
			return nil, fmt.Errorf("generate machine keys: %w", kErr)
		}
		slog.Info("Generated machine key pair.", "pubkey", pubKey)

		state = &State{
			Network: &network.Config{
				PrivateKey: privKey,
				PublicKey:  pubKey,
			},
		}
		state.SetPath(statePath)
		if err = state.Save(); err != nil {
			return nil, fmt.Errorf("save machine state: %w", err)
		}
	}

	corro, err := corrosion.NewAPIClient(config.CorrosionAPIAddr)
	if err != nil {
		return nil, fmt.Errorf("create corrosion API client: %w", err)
	}
	corroStore := store.New(corro)
	corroAdmin, err := corrosion.NewAdminClient(config.CorrosionAdminSockPath)
	if err != nil {
		return nil, fmt.Errorf("create corrosion admin client: %w", err)
	}
	c := cluster.NewCluster(corroStore, corroAdmin)

	// Init a gRPC Docker server that proxies requests to the local Docker daemon.
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create Docker client: %w", err)
	}
	dockerServer := docker.NewServer(dockerCli)

	// Init a local gRPC proxy server that proxies requests to the local or remote machine API servers.
	proxyDirector := apiproxy.NewDirector(config.MachineSockPath, APIPort)
	localProxyServer := grpc.NewServer(
		grpc.ForceServerCodecV2(proxy.Codec()),
		grpc.UnknownServiceHandler(
			proxy.TransparentHandler(proxyDirector.Director),
		),
	)

	m := &Machine{
		config:           *config,
		state:            state,
		started:          make(chan struct{}),
		initialised:      make(chan struct{}, 1),
		store:            corroStore,
		cluster:          c,
		docker:           dockerServer,
		localProxyServer: localProxyServer,
		proxyDirector:    proxyDirector,
	}
	m.localMachineServer = newGRPCServer(m, c, dockerServer)

	if m.Initialised() {
		m.initialised <- struct{}{}
	}

	return m, nil
}

func newGRPCServer(m pb.MachineServer, c pb.ClusterServer, d pb.DockerServer) *grpc.Server {
	s := grpc.NewServer()
	pb.RegisterMachineServer(s, m)
	pb.RegisterClusterServer(s, c)
	pb.RegisterDockerServer(s, d)
	return s
}

// Started returns a channel that is closed when the machine is ready to serve requests on the local API server.
func (m *Machine) Started() <-chan struct{} {
	return m.started
}

// Initialised returns true if the machine has been configured as a member of a cluster,
// either by initialising a new cluster on it or joining an existing one.
func (m *Machine) Initialised() bool {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()

	return m.state.ID != ""
}

func (m *Machine) Run(ctx context.Context) error {
	// Configure and start the corrosion service on the loopback if the machine is not initialised as a cluster
	// member. This provides the store required for the machine to initialise a new cluster on it. Once the machine
	// is initialised, the corrosion service is managed by the networkController.
	if !m.Initialised() {
		if err := m.configureCorrosion(); err != nil {
			return fmt.Errorf("configure corrosion service: %w", err)
		}
		slog.Info("Configured corrosion service.", "dir", m.config.CorrosionDir)

		if err := m.config.CorrosionService.Start(ctx); err != nil {
			return fmt.Errorf("start corrosion service: %w", err)
		}
	}

	// Use an errgroup to coordinate error handling and graceful shutdown of multiple machine components.
	errGroup, ctx := errgroup.WithContext(ctx)

	// Start the local machine API server.
	machineListener, err := listenUnixSocket(m.config.MachineSockPath)
	if err != nil {
		return fmt.Errorf("listen machine API unix socket %q: %w", m.config.MachineSockPath, err)
	}
	errGroup.Go(
		func() error {
			slog.Info("Starting local machine API server.", "path", m.config.MachineSockPath)
			if err := m.localMachineServer.Serve(machineListener); err != nil {
				return fmt.Errorf("local machine API server failed: %w", err)
			}
			return nil
		},
	)

	// Start the local API proxy server.
	proxyListener, err := listenUnixSocket(m.config.UncloudSockPath)
	if err != nil {
		return fmt.Errorf("listen API proxy unix socket %q: %w", m.config.UncloudSockPath, err)
	}
	errGroup.Go(
		func() error {
			slog.Info("Starting local API proxy server.", "path", m.config.UncloudSockPath)
			if err := m.localProxyServer.Serve(proxyListener); err != nil {
				return fmt.Errorf("local API proxy server failed: %w", err)
			}
			return nil
		},
	)
	// Signal that the machine is ready.
	close(m.started)

	// Control loop for managing components that depend on the machine being initialised as a cluster member.
	errGroup.Go(
		func() error {
			if !m.Initialised() {
				slog.Info(
					"Waiting for the machine to be initialised as a member of a cluster " +
						"to start the network controller.",
				)
			}

			var ctrl *networkController
			// Error channel for communicating the termination of the network controller.
			errCh := make(chan error)

			for {
				select {
				// Wait for the machine to be initialised as a member of a cluster to start the network controller.
				// It can be reset when leaving the cluster and then re-initialised again with a new configuration.
				case <-m.initialised:
					var err error

					m.cluster.UpdateMachineID(m.state.ID)

					// Ensure the corrosion config is up to date, including a new gossip address if the machine
					// has just joined a cluster.
					if err = m.configureCorrosion(); err != nil {
						return fmt.Errorf("configure corrosion service: %w", err)
					}
					slog.Info("Configured corrosion service.", "dir", m.config.CorrosionDir)

					slog.Info("Starting network controller.")
					// Update the proxy director's local address to the machine's management IP address, allowing
					// the proxy to identify which requests should be proxied to the local machine API server.
					m.proxyDirector.UpdateLocalAddress(m.state.Network.ManagementIP.String())
					proxyServer := grpc.NewServer(
						grpc.ForceServerCodecV2(proxy.Codec()),
						grpc.UnknownServiceHandler(
							proxy.TransparentHandler(m.proxyDirector.Director),
						),
					)

					ctrl, err = newNetworkController(m.state, m.store, proxyServer, m.config.CorrosionService)
					if err != nil {
						return fmt.Errorf("initialise network controller: %w", err)
					}

					go func() {
						if err = ctrl.Run(ctx); err != nil {
							errCh <- fmt.Errorf("run network controller: %w", err)
						} else {
							slog.Info("Network controller stopped.")
							errCh <- nil
						}
					}()
				case err := <-errCh:
					if err != nil {
						return err
					}
					ctrl = nil
				case <-ctx.Done():
					// Wait for the network controller to stop before returning.
					if ctrl != nil {
						if err := <-errCh; err != nil {
							return err
						}
					}
					return nil
				}
			}
		},
	)

	// Shutdown goroutine.
	errGroup.Go(
		func() error {
			<-ctx.Done()
			slog.Info("Stopping local machine API server.")
			// TODO: implement timeout for graceful shutdown.
			m.localMachineServer.GracefulStop()
			slog.Info("Local machine API server stopped.")

			slog.Info("Stopping local API proxy server.")
			// TODO: implement timeout for graceful shutdown.
			m.localProxyServer.GracefulStop()
			// Close the proxy director to close all backend connections.
			m.proxyDirector.Close()
			slog.Info("Local API proxy server stopped.")
			return nil
		},
	)

	return errGroup.Wait()
}

// listenUnixSocket creates a new Unix socket listener with the specified path. The socket file is created with 0660
// access mode and uncloud group if the group is found, otherwise it falls back to the root group.
func listenUnixSocket(path string) (net.Listener, error) {
	gid := 0 // Fall back to the root group if the uncloud group is not found.
	group, err := user.LookupGroup(DefaultUncloudSockGroup)
	if err != nil {
		//goland:noinspection GoTypeAssertionOnErrors
		if _, ok := err.(user.UnknownGroupError); ok {
			slog.Info(
				"Specified group not found, using root group for the API socket.",
				"group", DefaultUncloudSockGroup, "path", path,
			)
		} else {
			return nil, fmt.Errorf("lookup %q group ID (GID): %w", DefaultUncloudSockGroup, err)
		}
	} else {
		gid, err = strconv.Atoi(group.Gid)
		if err != nil {
			return nil, fmt.Errorf("parse %q group ID (GID) %q: %w", DefaultUncloudSockGroup, group.Gid, err)
		}
	}

	// Ensure the parent directory exists and has the correct group permissions.
	parent, _ := filepath.Split(path)
	if err = os.MkdirAll(parent, 0750); err != nil {
		return nil, fmt.Errorf("create directory %q: %w", parent, err)
	}
	if err = os.Chown(parent, -1, gid); err != nil {
		return nil, fmt.Errorf("chown directory %q: %w", parent, err)
	}

	return sockets.NewUnixSocket(path, gid)
}

func (m *Machine) configureCorrosion() error {
	if err := corroservice.MkDataDir(m.config.CorrosionDir, corroservice.DefaultUser); err != nil {
		return fmt.Errorf("create corrosion data directory: %w", err)
	}
	configPath := filepath.Join(m.config.CorrosionDir, "config.toml")
	schemaPath := filepath.Join(m.config.CorrosionDir, "schema.sql")

	// Use a loopback address as the gossip address (required) unless the machine has joined a cluster
	// and has a management IP.
	gossipAddr := netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), corroservice.DefaultGossipPort)
	if m.state.Network.ManagementIP.IsValid() {
		gossipAddr = netip.AddrPortFrom(m.state.Network.ManagementIP, corroservice.DefaultGossipPort)
	}
	// TODO: use a partial list of machine peers for bootstrapping if the cluster is large.
	var bootstrap []string
	for _, peer := range m.state.Network.Peers {
		if peer.Subnet == nil {
			// Skip non-machine peers.
			continue
		}
		bootstrap = append(bootstrap, netip.AddrPortFrom(peer.ManagementIP, corroservice.DefaultGossipPort).String())
	}
	cfg := corroservice.Config{
		DB: corroservice.DBConfig{
			Path:        filepath.Join(m.config.CorrosionDir, "store.db"),
			SchemaPaths: []string{schemaPath},
		},
		Gossip: corroservice.GossipConfig{
			Addr:      gossipAddr,
			Bootstrap: bootstrap,
			Plaintext: true,
		},
		API: corroservice.APIConfig{
			Addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), corroservice.DefaultAPIPort),
		},
		Admin: corroservice.AdminConfig{
			Path: filepath.Join(m.config.CorrosionDir, "admin.sock"),
		},
	}
	if err := cfg.Write(configPath, corroservice.DefaultUser); err != nil {
		return fmt.Errorf("write corrosion config: %w", err)
	}

	if err := os.WriteFile(schemaPath, []byte(store.Schema), 0644); err != nil {
		return fmt.Errorf("write corrosion schema: %w", err)
	}
	if err := corroservice.Chown(schemaPath, corroservice.DefaultUser); err != nil {
		return fmt.Errorf("chown corrosion schema: %w", err)
	}

	return nil
}

// InitCluster initialises a new cluster on the local machine with the provided network configuration.
func (m *Machine) InitCluster(ctx context.Context, req *pb.InitClusterRequest) (*pb.InitClusterResponse, error) {
	if m.Initialised() {
		return nil, status.Error(codes.FailedPrecondition, "machine is already configured as a cluster member")
	}

	clusterNetwork, err := req.Network.ToPrefix()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid network: %v", err)
	}

	if err = m.cluster.Init(ctx, clusterNetwork); err != nil {
		return nil, status.Errorf(codes.Internal, "init cluster: %v", err)
	}
	slog.Info("Cluster state initialised.", "network", clusterNetwork.String())

	machineName := req.MachineName
	if machineName == "" {
		if machineName, err = cluster.NewRandomMachineName(); err != nil {
			return nil, status.Errorf(codes.Internal, "generate machine name: %v", err)
		}
	}
	// Use the public and all routable IPs as endpoints.
	ips, err := network.ListRoutableIPs()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list routable IPs: %v", err)
	}
	publicIP, err := network.GetPublicIP()
	// Ignore the error if failed to get the public IP using API services.
	if err == nil {
		ips = append(ips, publicIP)
	}
	endpoints := make([]*pb.IPPort, len(ips))
	for i, addr := range ips {
		addrPort := netip.AddrPortFrom(addr, network.WireGuardPort)
		endpoints[i] = pb.NewIPPort(addrPort)
	}

	// Register the new machine in the cluster to populate the state and get its ID and subnet.
	// Public and private keys have already been initialised in the machine state when it was created.
	addReq := &pb.AddMachineRequest{
		Name: machineName,
		Network: &pb.NetworkConfig{
			Endpoints: endpoints,
			PublicKey: m.state.Network.PublicKey,
		},
	}
	addResp, err := m.cluster.AddMachine(ctx, addReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "add machine to cluster: %v", err)
	}

	subnet, err := addResp.Machine.Network.Subnet.ToPrefix()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	manageIP, err := addResp.Machine.Network.ManagementIp.ToAddr()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	// Update the machine state with the new cluster configuration.
	m.state.ID = addResp.Machine.Id
	m.state.Name = addResp.Machine.Name
	m.state.Network = &network.Config{
		Subnet:       subnet,
		ManagementIP: manageIP,
		PrivateKey:   m.state.Network.PrivateKey,
		PublicKey:    m.state.Network.PublicKey,
	}
	if err = m.state.Save(); err != nil {
		return nil, status.Errorf(codes.Internal, "save machine state: %v", err)
	}
	slog.Info("Cluster initialised with machine.", "id", m.state.ID, "machine", m.state.Name)
	// Signal that the machine is initialised as a member of a cluster.
	m.initialised <- struct{}{}

	resp := &pb.InitClusterResponse{
		Machine: addResp.Machine,
	}
	return resp, nil
}

// JoinCluster configures the local machine to join an existing cluster.
func (m *Machine) JoinCluster(_ context.Context, req *pb.JoinClusterRequest) (*emptypb.Empty, error) {
	if m.Initialised() {
		return nil, status.Error(codes.FailedPrecondition, "machine is already configured as a cluster member")
	}

	if req.Machine.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "machine ID not set")
	}
	if req.Machine.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "machine name not set")
	}
	if req.Machine.Network == nil {
		return nil, status.Error(codes.InvalidArgument, "network not set")
	}
	if err := req.Machine.Network.Validate(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid network config: %v", err)
	}
	if !m.state.Network.PublicKey.Equal(req.Machine.Network.PublicKey) {
		return nil, status.Error(
			codes.InvalidArgument, "public key in the request does not match the public key on the machine",
		)
	}

	// Update the machine state with the provided cluster configuration.
	subnet, _ := req.Machine.Network.Subnet.ToPrefix()
	manageIP, _ := req.Machine.Network.ManagementIp.ToAddr()
	m.state.ID = req.Machine.Id
	m.state.Name = req.Machine.Name
	m.state.Network = &network.Config{
		Subnet:       subnet,
		ManagementIP: manageIP,
		PrivateKey:   m.state.Network.PrivateKey,
		PublicKey:    m.state.Network.PublicKey,
	}

	// Build a peers config from other cluster machines.
	m.state.Network.Peers = make([]network.PeerConfig, 0, len(req.OtherMachines))
	for _, om := range req.OtherMachines {
		if err := om.Network.Validate(); err != nil {
			continue
		}
		omSubnet, _ := om.Network.Subnet.ToPrefix()
		omManageIP, _ := om.Network.ManagementIp.ToAddr()
		omEndpoints := make([]netip.AddrPort, len(om.Network.Endpoints))
		for i, ep := range om.Network.Endpoints {
			addrPort, _ := ep.ToAddrPort()
			omEndpoints[i] = addrPort
		}
		peer := network.PeerConfig{
			Subnet:       &omSubnet,
			ManagementIP: omManageIP,
			AllEndpoints: omEndpoints,
			PublicKey:    om.Network.PublicKey,
		}
		if len(omEndpoints) > 0 {
			peer.Endpoint = &omEndpoints[0]
		}
		m.state.Network.Peers = append(m.state.Network.Peers, peer)
	}

	if err := m.state.Save(); err != nil {
		return nil, status.Errorf(codes.Internal, "save machine state: %v", err)
	}
	slog.Info("Machine configured to join the cluster.", "id", m.state.ID, "name", m.state.Name)
	// Signal that the machine is initialised as a member of a cluster.
	m.initialised <- struct{}{}

	return &emptypb.Empty{}, nil
}

// Token returns the local machine's token that can be used for adding the machine to a cluster.
func (m *Machine) Token(_ context.Context, _ *emptypb.Empty) (*pb.TokenResponse, error) {
	if len(m.state.Network.PublicKey) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "public key is not set in machine state")
	}

	ips, err := network.ListRoutableIPs()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list routable IPs: %v", err)
	}
	publicIP, err := network.GetPublicIP()
	// Ignore the error if failed to get the public IP using API services.
	if err == nil {
		ips = append(ips, publicIP)
	}
	endpoints := make([]netip.AddrPort, len(ips))
	for i, ip := range ips {
		endpoints[i] = netip.AddrPortFrom(ip, network.WireGuardPort)
	}

	token := NewToken(m.state.Network.PublicKey, endpoints)
	tokenStr, err := token.String()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.TokenResponse{Token: tokenStr}, nil
}

func (m *Machine) Inspect(_ context.Context, _ *emptypb.Empty) (*pb.MachineInfo, error) {
	return &pb.MachineInfo{
		Id:   m.state.ID,
		Name: m.state.Name,
		Network: &pb.NetworkConfig{
			Subnet:       pb.NewIPPrefix(m.state.Network.Subnet),
			ManagementIp: pb.NewIP(m.state.Network.ManagementIP),
			PublicKey:    m.state.Network.PublicKey,
		},
	}, nil
}
