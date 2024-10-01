package machine

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/go-connections/sockets"
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
	"uncloud/internal/machine/cluster"
	"uncloud/internal/machine/corroservice"
	"uncloud/internal/machine/network"
	"uncloud/internal/machine/store"
)

const (
	DefaultAPISockPath  = "/run/uncloud.sock"
	DefaultAPISockGroup = "uncloud"
)

type Config struct {
	// DataDir is the directory where the machine stores its persistent state.
	DataDir     string
	APISockPath string

	CorrosionDir           string
	CorrosionAPIListenAddr netip.AddrPort
	CorrosionAPIAddr       netip.AddrPort
	CorrosionService       corroservice.Service
}

// SetDefaults returns a new Config with default values set where not provided.
func (c *Config) SetDefaults() *Config {
	// Copy c into a new Config to avoid modifying the original.
	cfg := *c

	if cfg.DataDir == "" {
		cfg.DataDir = "/var/lib/uncloud"
	}
	if cfg.APISockPath == "" {
		cfg.APISockPath = DefaultAPISockPath
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

	localServer   *grpc.Server
	cluster       *cluster.Cluster
	newMachinesCh <-chan *pb.MachineInfo
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

	var c *cluster.Cluster
	clusterState := cluster.NewState(cluster.StatePath(config.DataDir))
	if err = clusterState.Load(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Cluster state file does not exist, initialise the cluster without a state to fail cluster requests.
			c = cluster.NewCluster(nil, corroStore)
		} else {
			return nil, fmt.Errorf("load cluster state: %w", err)
		}
	} else {
		// Cluster state is successfully loaded, initialise the cluster with it.
		c = cluster.NewCluster(clusterState, corroStore)
	}

	m := &Machine{
		config:        *config,
		state:         state,
		started:       make(chan struct{}),
		initialised:   make(chan struct{}, 1),
		cluster:       c,
		newMachinesCh: c.WatchNewMachines(),
	}
	m.localServer = newGRPCServer(m, c)

	if m.IsInitialised() {
		m.initialised <- struct{}{}
	}

	return m, nil
}

func newGRPCServer(m pb.MachineServer, c pb.ClusterServer) *grpc.Server {
	s := grpc.NewServer()
	pb.RegisterMachineServer(s, m)
	pb.RegisterClusterServer(s, c)
	return s
}

// Started returns a channel that is closed when the machine is ready to serve requests on the local API server.
func (m *Machine) Started() <-chan struct{} {
	return m.started
}

// IsInitialised returns true if the machine has been configured as a member of a cluster,
// either by initialising a new cluster on it or joining an existing one.
func (m *Machine) IsInitialised() bool {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()

	return m.state.ID != ""
}

func (m *Machine) Run(ctx context.Context) error {
	// Use an errgroup to coordinate error handling and graceful shutdown of multiple machine components.
	errGroup, ctx := errgroup.WithContext(ctx)

	// Start the machine local API server.
	localListener, err := listenUnixSocket(m.config.APISockPath)
	if err != nil {
		return fmt.Errorf("listen API unix socket %q: %w", m.config.APISockPath, err)
	}
	errGroup.Go(
		func() error {
			slog.Info("Starting local API server.", "path", m.config.APISockPath)
			if err := m.localServer.Serve(localListener); err != nil {
				return fmt.Errorf("local API server failed: %w", err)
			}
			return nil
		},
	)
	close(m.started)

	// Control loop for managing the network controller.
	errGroup.Go(
		func() error {
			if !m.IsInitialised() {
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
					slog.Info("Starting network controller.")
					networkServer := newGRPCServer(m, m.cluster)

					if err = m.configureCorrosion(); err != nil {
						return fmt.Errorf("configure corrosion service: %w", err)
					}

					ctrl, err = newNetworkController(m.state, networkServer, m.config.CorrosionService, m.newMachinesCh)
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
			slog.Info("Stopping local API server.")
			// TODO: implement timeout for graceful shutdown.
			m.localServer.GracefulStop()
			slog.Info("Local API server stopped.")
			return nil
		},
	)

	return errGroup.Wait()
}

// listenUnixSocket creates a new Unix socket listener with the specified path. The socket file is created with 0660
// access mode and uncloud group if the group is found, otherwise it falls back to the root group.
func listenUnixSocket(path string) (net.Listener, error) {
	gid := 0 // Fall back to the root group if the uncloud group is not found.
	group, err := user.LookupGroup(DefaultAPISockGroup)
	if err != nil {
		if _, ok := err.(user.UnknownGroupError); ok {
			slog.Info(
				"Specified group not found, using root group for the API socket.", "group", DefaultAPISockGroup, "path",
				path,
			)
		} else {
			return nil, fmt.Errorf("lookup %q group ID (GID): %w", DefaultAPISockGroup, err)
		}
	} else {
		gid, err = strconv.Atoi(group.Gid)
		if err != nil {
			return nil, fmt.Errorf("parse %q group ID (GID) %q: %w", DefaultAPISockGroup, group.Gid, err)
		}
	}

	return sockets.NewUnixSocket(path, gid)
}

func (m *Machine) configureCorrosion() error {
	if err := corroservice.MkDataDir(m.config.CorrosionDir, corroservice.DefaultUser); err != nil {
		return fmt.Errorf("create corrosion data directory: %w", err)
	}
	configPath := filepath.Join(m.config.CorrosionDir, "config.toml")
	schemaPath := filepath.Join(m.config.CorrosionDir, "schema.sql")

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
			Addr:      netip.AddrPortFrom(m.state.Network.ManagementIP, corroservice.DefaultGossipPort),
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

// InitCluster resets the local machine and initialises a new cluster with it.
func (m *Machine) InitCluster(ctx context.Context, req *pb.InitClusterRequest) (*pb.InitClusterResponse, error) {
	var err error
	machineName := req.MachineName
	if machineName == "" {
		machineName, err = cluster.NewRandomMachineName()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "generate machine name: %v", err)
		}
	}

	// TODO: a proper cluster leave mechanism and machine reset should be implemented later.
	//  For now just reset the machine state and cluster state.
	clusterStatePath := cluster.StatePath(m.config.DataDir)
	clusterState := cluster.NewState(clusterStatePath)
	if err = clusterState.Save(); err != nil {
		return nil, status.Errorf(codes.Internal, "save cluster state: %v", err)
	}
	m.cluster.SetState(clusterState)
	slog.Info("Cluster state initialised.", "path", clusterStatePath)
	if err = m.cluster.SetNetwork(req.Network); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "set cluster network: %v", err)
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

	// Add a user to the cluster and build a peers config from it if provided.
	if req.User != nil {
		if err = m.cluster.AddUser(req.User); err != nil {
			return nil, status.Errorf(codes.Internal, "add user to cluster: %v", err)
		}
		userManageIP, uErr := req.User.Network.ManagementIp.ToAddr()
		if uErr != nil {
			return nil, status.Error(codes.Internal, uErr.Error())
		}

		m.state.Network.Peers = make([]network.PeerConfig, 1)
		m.state.Network.Peers[0] = network.PeerConfig{
			ManagementIP: userManageIP,
			PublicKey:    req.User.Network.PublicKey,
		}
	}

	if err = m.state.Save(); err != nil {
		return nil, status.Errorf(codes.Internal, "save machine state: %v", err)
	}
	slog.Info("Cluster initialised with machine.", "machine", m.state.Name)
	// Signal that the machine is initialised as a member of a cluster.
	m.initialised <- struct{}{}

	resp := &pb.InitClusterResponse{
		Machine: addResp.Machine,
	}
	return resp, nil
}

// JoinCluster resets the local machine and configures it to join an existing cluster.
func (m *Machine) JoinCluster(ctx context.Context, req *pb.JoinClusterRequest) (*emptypb.Empty, error) {
	// TODO: a proper cluster leave mechanism and machine reset should be implemented later.
	//  For now assume the machine wasn't part of a cluster.

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
