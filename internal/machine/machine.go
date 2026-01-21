package machine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"sync"

	"github.com/docker/docker/client"
	"github.com/docker/go-connections/sockets"
	"github.com/psviderski/uncloud/internal/corrosion"
	"github.com/psviderski/uncloud/internal/docker"
	"github.com/psviderski/uncloud/internal/fs"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	apiproxy "github.com/psviderski/uncloud/internal/machine/api/proxy"
	"github.com/psviderski/uncloud/internal/machine/caddyconfig"
	"github.com/psviderski/uncloud/internal/machine/cluster"
	"github.com/psviderski/uncloud/internal/machine/constants"
	"github.com/psviderski/uncloud/internal/machine/corroservice"
	"github.com/psviderski/uncloud/internal/machine/dns"
	machinedocker "github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/unregistry"
	"github.com/siderolabs/grpc-proxy/proxy"
	"golang.org/x/sync/errgroup"
	"golang.zx2c4.com/wireguard/wgctrl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	DefaultMachineSockPath = "/run/uncloud/machine.sock"
	DefaultUncloudSockPath = "/run/uncloud/uncloud.sock"
	DefaultSockGroup       = "uncloud"
	// DefaultCaddyAdminSockPath is the default path to the Caddy admin socket for validating the generated Caddy
	// reverse proxy configuration.
	DefaultCaddyAdminSockPath = "/run/uncloud/caddy/admin.sock"
)

type Config struct {
	// DataDir is the directory where the machine stores its persistent state. Default is /var/lib/uncloud.
	DataDir         string
	MachineSockPath string
	UncloudSockPath string

	CorrosionDir           string
	CorrosionAPIListenAddr netip.AddrPort
	CorrosionAPIAddr       netip.AddrPort
	CorrosionAdminSockPath string
	CorrosionService       corroservice.Service
	// CorrosionUser sets the Linux user for running the corrosion service.
	CorrosionUser string

	// DockerClient manages system and user containers using the local Docker daemon.
	DockerClient *client.Client
	// ContainerdSockPath is the path to the containerd.sock used by Docker.
	ContainerdSockPath string

	// CaddyConfigDir specifies the directory where the machine generates the Caddy reverse proxy configuration file
	// for routing external traffic to service containers across the internal network. Default is DataDir/caddy.
	CaddyConfigDir string
	// DNSUpstreams specifies the upstream DNS servers for the embedded internal DNS server.
	DNSUpstreams []netip.AddrPort
}

// SetDefaults returns a new Config with default values set where not provided.
func (c *Config) SetDefaults() (*Config, error) {
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

	if cfg.DockerClient == nil {
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return nil, fmt.Errorf("create Docker client: %w", err)
		}
		cfg.DockerClient = cli
	}
	if cfg.ContainerdSockPath == "" {
		// Auto-detect the containerd.sock path used by Docker.
		paths := []string{
			"/run/containerd/containerd.sock", // Default path on most Linux distributions.
			"/run/docker/containerd/containerd.sock",
			"/var/run/containerd/containerd.sock",
			"/var/run/docker/containerd/containerd.sock",
		}
		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				cfg.ContainerdSockPath = path
				slog.Debug("Detected containerd socket used by Docker.", "path", path)
				break
			}
		}

		if cfg.ContainerdSockPath == "" {
			slog.Warn("Failed to auto-detect containerd socket used by Docker.")
		}
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
	if cfg.CorrosionUser == "" {
		cfg.CorrosionUser = corroservice.DefaultUser
	}
	if cfg.CorrosionService == nil {
		if isRunningInDocker() {
			// Run corrosion in a nested Docker container if the machine is running in a container.
			uid, gid, err := fs.LookupUIDGID(cfg.CorrosionUser)
			if err != nil {
				return nil, fmt.Errorf("lookup corrosion user %q: %w", cfg.CorrosionUser, err)
			}

			cfg.CorrosionService = &corroservice.DockerService{
				Client:  cfg.DockerClient,
				Image:   corroservice.LatestImage,
				Name:    "uncloud-corrosion",
				DataDir: cfg.CorrosionDir,
				User:    fmt.Sprintf("%d:%d", uid, gid),
			}
		} else {
			cfg.CorrosionService = corroservice.DefaultSystemdService(cfg.CorrosionDir)
		}
	}

	if cfg.CaddyConfigDir == "" {
		cfg.CaddyConfigDir = filepath.Join(cfg.DataDir, "caddy")
	}

	return &cfg, nil
}

// isRunningInDocker returns true if the current process is running in a Docker container.
func isRunningInDocker() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

type Machine struct {
	pb.UnimplementedMachineServer

	config Config
	state  *State
	// started is closed when the machine is ready to serve requests on the local API server.
	started chan struct{}
	// initialised is closed when the machine is configured as a member of a cluster.
	initialised chan struct{}
	// networkReady is closed when the Docker network is configured and ready for containers.
	networkReady chan struct{}
	// clusterReady is closed when the cluster controller has finished starting all components
	// and the machine is ready to serve cluster requests.
	clusterReady chan struct{}
	// resetting is true when the machine is being reset.
	resetting bool
	// stop cancels the Run method context to stop the machine gracefully.
	stop func()

	clusterCtrl *clusterController
	// store is the cluster store backed by a distributed Corrosion database.
	store   *store.Store
	cluster *cluster.Cluster
	// dockerService provides high-level operations for managing Docker containers.
	dockerService *machinedocker.Service
	dockerServer  *machinedocker.Server
	// localMachineServer is the gRPC server for the machine API listening on the local Unix socket.
	localMachineServer *grpc.Server

	// proxyDirector manages routing of gRPC requests between local and remote machine API servers.
	proxyDirector *apiproxy.Director
	// localProxyServer is the gRPC proxy server for the machine API listening on the local Unix socket.
	// It proxies requests to the local or remote machine API servers depending on the request targets
	// and aggregates responses.
	localProxyServer *grpc.Server

	// mu protects the Machine from concurrent reads and writes.
	mu sync.RWMutex
}

func NewMachine(config *Config) (*Machine, error) {
	config, err := config.SetDefaults()
	if err != nil {
		return nil, fmt.Errorf("set default config values: %w", err)
	}

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

	initialised := make(chan struct{})
	clusterReady := make(chan struct{})
	c := cluster.NewCluster(corroStore, corroAdmin, initialised, clusterReady)

	// Init dependencies for a gRPC Docker server that proxies requests to the local Docker daemon.
	dbFilePath := filepath.Join(config.DataDir, DBFileName)
	db, err := NewDB(dbFilePath)
	if err != nil {
		return nil, fmt.Errorf("init machine database: %w", err)
	}
	dockerService := machinedocker.NewService(config.DockerClient, db)

	// Init a local gRPC proxy server that proxies requests to the local or remote machine API servers.
	proxyDirector := apiproxy.NewDirector(config.MachineSockPath, constants.MachineAPIPort)
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
		initialised:      initialised,
		networkReady:     make(chan struct{}),
		clusterReady:     clusterReady,
		store:            corroStore,
		cluster:          c,
		dockerService:    dockerService,
		localProxyServer: localProxyServer,
		proxyDirector:    proxyDirector,
	}

	// Machine IP will only be available after the machine is initialised as a cluster member so wrap it in a function.
	internalDNSIP := func() netip.Addr {
		return m.IP()
	}
	// Machine ID will only be available after the machine is initialised as a cluster member so wrap it in a function.
	machineID := func() string {
		return m.state.ID
	}
	m.dockerServer = machinedocker.NewServer(dockerService, db, internalDNSIP, machineID, machinedocker.ServerOptions{
		NetworkReady:        m.IsNetworkReady,
		WaitForNetworkReady: m.WaitForNetworkReady,
	})
	caddyServer := caddyconfig.NewServer(caddyconfig.NewService(config.CaddyConfigDir))
	m.localMachineServer = newGRPCServer(m, c, m.dockerServer, caddyServer)

	if m.Initialised() {
		close(m.initialised)
	}

	return m, nil
}

func newGRPCServer(m pb.MachineServer, c pb.ClusterServer, d pb.DockerServer, caddy pb.CaddyServer) *grpc.Server {
	s := grpc.NewServer()
	pb.RegisterMachineServer(s, m)
	pb.RegisterClusterServer(s, c)
	pb.RegisterDockerServer(s, d)
	pb.RegisterCaddyServer(s, caddy)
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

// IP returns the machine IPv4 address in the cluster network which is the first address in the machine subnet.
func (m *Machine) IP() netip.Addr {
	if !m.Initialised() {
		return netip.Addr{}
	}

	return network.MachineIP(m.state.Network.Subnet)
}

func (m *Machine) Run(ctx context.Context) error {
	// Create a cancellable context for the Run method to allow stopping the machine gracefully.
	ctx, m.stop = context.WithCancel(ctx)

	// Docker dependency is essential for the machine to function. Block until it's ready.
	if err := docker.WaitDaemonReady(ctx, m.config.DockerClient); err != nil {
		return fmt.Errorf("wait for Docker daemon: %w", err)
	}

	// Configure and start the corrosion service on the loopback if the machine is not initialised as a cluster
	// member. This provides the store required for the machine to initialise a new cluster on it. Once the machine
	// is initialised, the corrosion service is managed by the clusterController.
	if !m.Initialised() {
		if err := m.configureCorrosion(); err != nil {
			return fmt.Errorf("configure corrosion service: %w", err)
		}
		slog.Info("Configured corrosion service.", "dir", m.config.CorrosionDir)

		if err := m.config.CorrosionService.Start(ctx); err != nil {
			return fmt.Errorf("start corrosion service: %w", err)
		}
		slog.Info("Corrosion service started.")
	}

	// Use an errgroup to coordinate error handling and graceful shutdown of multiple machine components.
	errGroup, ctx := errgroup.WithContext(ctx)

	// Start the local machine API server.
	machineListener, err := listenUnixSocket(m.config.MachineSockPath)
	if err != nil {
		return fmt.Errorf("listen machine API unix socket %q: %w", m.config.MachineSockPath, err)
	}
	errGroup.Go(func() error {
		slog.Info("Starting local machine API server.", "path", m.config.MachineSockPath)
		if err := m.localMachineServer.Serve(machineListener); err != nil {
			return fmt.Errorf("local machine API server failed: %w", err)
		}
		return nil
	})

	// Start the local API proxy server.
	proxyListener, err := listenUnixSocket(m.config.UncloudSockPath)
	if err != nil {
		return fmt.Errorf("listen API proxy unix socket %q: %w", m.config.UncloudSockPath, err)
	}
	errGroup.Go(func() error {
		slog.Info("Starting local API proxy server.", "path", m.config.UncloudSockPath)
		if err := m.localProxyServer.Serve(proxyListener); err != nil {
			return fmt.Errorf("local API proxy server failed: %w", err)
		}
		return nil
	})
	// Signal that the machine is ready.
	close(m.started)

	// Wait for the machine to be initialised as a member of a cluster and run the cluster controller.
	errGroup.Go(func() error {
		if !m.Initialised() {
			slog.Info(
				"Waiting for the machine to be initialised as a member of a cluster to start the cluster controller.",
			)
		}

		select {
		case <-m.initialised:
			m.cluster.UpdateMachineID(m.state.ID)

			// Ensure the corrosion config is up to date, including a new gossip address if the machine
			// has just joined a cluster.
			if err := m.configureCorrosion(); err != nil {
				return fmt.Errorf("configure corrosion service: %w", err)
			}
			slog.Info("Configured corrosion service.", "dir", m.config.CorrosionDir)

			slog.Info("Starting cluster controller.")
			// Update the proxy director's local address to the machine's management IP address, allowing
			// the proxy to identify which requests should be proxied to the local machine API server.
			m.proxyDirector.UpdateLocalAddress(m.state.Network.ManagementIP.String())
			proxyServer := grpc.NewServer(
				grpc.ForceServerCodecV2(proxy.Codec()),
				grpc.UnknownServiceHandler(
					proxy.TransparentHandler(m.proxyDirector.Director),
				),
			)

			// Create a new caddyconfig controller for managing the Caddy reverse proxy configuration.
			// It will also serve the current machine ID at /.uncloud-verify to verify Caddy reachability.
			caddyconfigCtrl, err := caddyconfig.NewController(
				m.state.ID,
				m.config.CaddyConfigDir,
				DefaultCaddyAdminSockPath,
				m.store,
			)
			if err != nil {
				return fmt.Errorf("create caddyconfig controller: %w", err)
			}

			dnsResolver := dns.NewClusterResolver(m.store)
			dnsServer, err := dns.NewServer(
				m.IP(),
				m.state.Network.Subnet,
				dnsResolver,
				m.config.DNSUpstreams,
			)
			if err != nil {
				return fmt.Errorf("create embedded DNS server: %w", err)
			}

			var unreg *unregistry.Registry
			if m.config.ContainerdSockPath != "" {
				isContainerdStore, err := m.dockerService.IsContainerdImageStoreEnabled(ctx)
				if err != nil {
					return fmt.Errorf("check if Docker uses containerd image store: %w", err)
				}

				if isContainerdStore {
					// Create an embedded container registry listening on the machine IP address and
					// using the local Docker (containerd) image store as its backend.
					unreg, err = unregistry.NewRegistry(unregistry.Config{
						Addr:                net.JoinHostPort(m.IP().String(), strconv.Itoa(constants.UnregistryPort)),
						ContainerdNamespace: "moby",
						ContainerdSock:      m.config.ContainerdSockPath,
						LogFormatter:        "text",
						LogLevel:            "info",
					})
					if err != nil {
						return fmt.Errorf("create embedded registry: %w", err)
					}
				} else {
					slog.Warn("Skipping embedded unregistry setup as Docker is not using the containerd image store.")
				}
			} else {
				slog.Warn("Skipping embedded unregistry setup as the containerd socket path is not configured.")
			}

			m.mu.Lock()
			m.clusterCtrl, err = newClusterController(
				m.state,
				m.store,
				proxyServer,
				m.config.CorrosionService,
				m.dockerService,
				m.networkReady,
				m.clusterReady,
				caddyconfigCtrl,
				dnsServer,
				dnsResolver,
				unreg,
			)
			m.mu.Unlock()
			if err != nil {
				return fmt.Errorf("initialise cluster controller: %w", err)
			}

			if err = m.clusterCtrl.Run(ctx); err != nil {
				return fmt.Errorf("run cluster controller: %w", err)
			}
			slog.Info("Cluster controller stopped.")

		case <-ctx.Done():
			// The context was cancelled before the machine was initialised.
		}

		return nil
	})

	// Shutdown goroutine.
	errGroup.Go(func() error {
		var err error

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

		// Clean up the machine data and resources if the machine shutdown was initiated by a reset.
		if m.resetting {
			slog.Info("Cleaning up machine data and resources.")
			if err = m.cleanup(); err != nil {
				slog.Error("Failed to clean up machine data and resources.", "err", err)
			}
		}

		m.config.DockerClient.Close()
		return err
	})

	return errGroup.Wait()
}

// listenUnixSocket creates a new Unix socket listener with the specified path. The socket file is created with 0660
// access mode and uncloud group if the group is found, otherwise it falls back to the root group.
func listenUnixSocket(path string) (net.Listener, error) {
	gid := 0 // Fall back to the root group if the uncloud group is not found.
	group, err := user.LookupGroup(DefaultSockGroup)
	if err != nil {
		//goland:noinspection GoTypeAssertionOnErrors
		if _, ok := err.(user.UnknownGroupError); ok {
			slog.Info(
				"Specified group not found, using root group for the API socket.",
				"group", DefaultSockGroup, "path", path,
			)
		} else {
			return nil, fmt.Errorf("lookup %q group ID (GID): %w", DefaultSockGroup, err)
		}
	} else {
		gid, err = strconv.Atoi(group.Gid)
		if err != nil {
			return nil, fmt.Errorf("parse %q group ID (GID) %q: %w", DefaultSockGroup, group.Gid, err)
		}
	}

	// Ensure the parent directory exists and has the correct group permissions.
	parent, _ := filepath.Split(path)
	if err = os.MkdirAll(parent, 0o750); err != nil {
		return nil, fmt.Errorf("create directory %q: %w", parent, err)
	}
	if err = os.Chown(parent, -1, gid); err != nil {
		return nil, fmt.Errorf("chown directory %q: %w", parent, err)
	}

	return sockets.NewUnixSocket(path, gid)
}

func (m *Machine) configureCorrosion() error {
	if err := corroservice.MkDataDir(m.config.CorrosionDir, m.config.CorrosionUser); err != nil {
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
			Addr: m.config.CorrosionAPIAddr,
		},
		Admin: corroservice.AdminConfig{
			Path: filepath.Join(m.config.CorrosionDir, "admin.sock"),
		},
	}
	// TODO: change file permissions to 0640 root:uncloud to emphasize the owner is the machine, not corrosion.
	if err := cfg.Write(configPath, m.config.CorrosionUser); err != nil {
		return fmt.Errorf("write corrosion config: %w", err)
	}

	if err := os.WriteFile(schemaPath, []byte(store.Schema), 0o644); err != nil {
		return fmt.Errorf("write corrosion schema: %w", err)
	}

	return nil
}

// cleanup removes the machine resources and persistent state.
func (m *Machine) cleanup() error {
	var errs []error

	m.mu.RLock()
	clusterCtrl := m.clusterCtrl
	m.mu.RUnlock()
	if clusterCtrl != nil {
		if err := clusterCtrl.Cleanup(); err != nil {
			errs = append(errs, fmt.Errorf("cleanup cluster resources: %w", err))
		}
	}

	if err := os.RemoveAll(m.config.DataDir); err != nil {
		errs = append(errs,
			fmt.Errorf("remove data directory with persistent machine state '%s': %w", m.config.DataDir, err))
	} else {
		slog.Info("Removed data directory storing persistent machine state.", "path", m.config.DataDir)
	}

	return errors.Join(errs...)
}

// CheckPrerequisites verifies if the machine meets all necessary system requirements to participate in the cluster.
func (m *Machine) CheckPrerequisites(_ context.Context, _ *emptypb.Empty) (*pb.CheckPrerequisitesResponse, error) {
	// Check DNS port (UDP) availability.
	if err := checkDNSPortAvailable(); err != nil {
		return &pb.CheckPrerequisitesResponse{
			Satisfied: false,
			Error:     err.Error(),
		}, nil
	}

	return &pb.CheckPrerequisitesResponse{
		Satisfied: true,
	}, nil
}

// checkDNSPortAvailable verifies that DNS port 53/udp is available for Uncloud's embedded DNS service.
func checkDNSPortAvailable() error {
	addr := &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 210), // Use a unique loopback address to avoid conflicts.
		Port: dns.Port,
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("DNS port %d/udp is already in use by another service: %w. Uncloud needs this port "+
			"to run the embedded internal DNS service on WireGuard interface 'uncloud'. Please reconfigure "+
			"any DNS servers (like dnsmasq, systemd-resolved, or named) that might be listening on all network "+
			"interfaces (0.0.0.0) on the machine and try again", dns.Port, err)
	}
	conn.Close()
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
	publicIP, pubIPErr := network.GetPublicIP()
	// Ignore the error if failed to get the public IP using API services.
	if pubIPErr == nil && !slices.Contains(ips, publicIP) {
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
	if req.GetPublicIp() != nil {
		addReq.PublicIp = req.GetPublicIp()
	} else if req.GetPublicIpAuto() && pubIPErr == nil {
		addReq.PublicIp = pb.NewIP(publicIP)
	}

	addResp, err := m.cluster.AddMachineWithoutReadyCheck(ctx, addReq)
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
	close(m.initialised)

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
	m.state.MinStoreDBVersion = req.MinStoreDbVersion

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
	slog.Info(
		"Machine configured to join the cluster.",
		"id", m.state.ID,
		"name", m.state.Name,
		"subnet", m.state.Network.Subnet.String(),
		"management_ip", m.state.Network.ManagementIP.String(),
		"peers", len(m.state.Network.Peers),
	)
	// Signal that the machine is initialised as a member of a cluster.
	close(m.initialised)

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
	if err == nil && !slices.Contains(ips, publicIP) {
		ips = append(ips, publicIP)
	}
	endpoints := make([]netip.AddrPort, len(ips))
	for i, ip := range ips {
		endpoints[i] = netip.AddrPortFrom(ip, network.WireGuardPort)
	}

	token := NewToken(m.state.Network.PublicKey, publicIP, endpoints)
	tokenStr, err := token.String()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.TokenResponse{Token: tokenStr}, nil
}

// Deprecated: use InspectMachine instead.
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

func (m *Machine) InspectMachine(ctx context.Context, _ *emptypb.Empty) (*pb.InspectMachineResponse, error) {
	dbVersion, err := m.store.DBVersion(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get database version of the cluster store: %v", err)
	}

	return &pb.InspectMachineResponse{
		Machines: []*pb.MachineDetails{
			{
				// Metadata is injected by the gRPC proxy.
				Machine: &pb.MachineInfo{
					Id:   m.state.ID,
					Name: m.state.Name,
					Network: &pb.NetworkConfig{
						Subnet:       pb.NewIPPrefix(m.state.Network.Subnet),
						ManagementIp: pb.NewIP(m.state.Network.ManagementIP),
						PublicKey:    m.state.Network.PublicKey,
					},
				},
				StoreDbVersion: dbVersion,
			},
		},
	}, nil
}

// IsNetworkReady returns true if the Docker network is ready for containers.
func (m *Machine) IsNetworkReady() bool {
	if !m.Initialised() {
		// If machine is not initialized, there's no network to check
		return false
	}

	// Check if network is ready by checking if the networkReady channel has been closed
	select {
	case <-m.networkReady:
		return true
	default:
		return false
	}
}

// WaitForNetworkReady waits for the Docker network to be ready for containers.
// It returns nil when the network is ready or an error if the context is cancelled.
func (m *Machine) WaitForNetworkReady(ctx context.Context) error {
	if !m.Initialised() {
		// If machine is not initialized, there's no network to wait for
		return nil
	}

	// Wait for network to be ready or context to be cancelled
	select {
	case <-m.networkReady:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// InspectWireGuardNetwork retrieves the current WireGuard network configuration and peer status.
func (m *Machine) InspectWireGuardNetwork(
	_ context.Context, _ *emptypb.Empty,
) (*pb.InspectWireGuardNetworkResponse, error) {
	deviceName := network.WireGuardInterfaceName

	wg, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("create WireGuard client: %w", err)
	}
	defer wg.Close()

	dev, err := wg.Device(deviceName)
	if err != nil {
		return nil, fmt.Errorf("get WireGuard device '%s': %w", deviceName, err)
	}

	peers := make([]*pb.WireGuardPeer, len(dev.Peers))
	for i, p := range dev.Peers {
		var lastHandshake *timestamppb.Timestamp
		if !p.LastHandshakeTime.IsZero() {
			lastHandshake = timestamppb.New(p.LastHandshakeTime)
		}

		allowedIPs := make([]string, len(p.AllowedIPs))
		for j, ip := range p.AllowedIPs {
			allowedIPs[j] = ip.String()
		}

		var endpoint string
		if p.Endpoint != nil {
			endpoint = p.Endpoint.String()
		}

		peers[i] = &pb.WireGuardPeer{
			PublicKey:         p.PublicKey[:],
			Endpoint:          endpoint,
			LastHandshakeTime: lastHandshake,
			ReceiveBytes:      p.ReceiveBytes,
			TransmitBytes:     p.TransmitBytes,
			AllowedIps:        allowedIPs,
		}
	}

	return &pb.InspectWireGuardNetworkResponse{
		InterfaceName: dev.Name,
		PublicKey:     dev.PublicKey[:],
		ListenPort:    int32(dev.ListenPort),
		Peers:         peers,
	}, nil
}

// Reset restores the machine to a clean state, scheduling a graceful shutdown and removing all cluster-related
// configuration and resource. The uncloud daemon will restart the machine if managed by systemd.
func (m *Machine) Reset(_ context.Context, _ *pb.ResetRequest) (*emptypb.Empty, error) {
	if !m.Initialised() {
		return nil, nil
	}

	// Check if the machine is already being reset to avoid concurrent resets.
	m.mu.Lock()
	if m.resetting {
		m.mu.Unlock()
		return nil, status.Error(codes.FailedPrecondition, "machine is already being reset")
	}
	m.resetting = true
	m.mu.Unlock()

	slog.Info("Resetting machine to a clean state.")
	// Trigger the machine shutdown. The resetting boolean informs the machine to clean up its resources on shutdown.
	// We can't clean up the resources synchronously here because this is an RPC call that depends on the running
	// gRPC server and network.
	m.stop()

	return &emptypb.Empty{}, nil
}

// InspectService returns detailed information about a service and its containers stored in the cluster store.
func (m *Machine) InspectService(
	ctx context.Context, req *pb.InspectServiceRequest,
) (*pb.InspectServiceResponse, error) {
	opts := store.ListOptions{ServiceIDOrName: store.ServiceIDOrNameOptions{
		ID:   req.Id,
		Name: req.Id,
	}}

	records, err := m.store.ListContainers(ctx, opts)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list containers: %v", err)
	}
	if len(records) == 0 {
		return nil, status.Error(codes.NotFound, "service not found")
	}
	// TODO: handle SyncStatus to return only trusted container statuses.
	// TODO: handle multiple services with the same name but different IDs. This can happen when two services
	//  with the same name are created concurrently on different machines.

	containers := make([]*pb.Service_Container, len(records))
	for i, r := range records {
		containerJSON, err := json.Marshal(r.Container)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "marshal container: %v", err)
		}
		containers[i] = &pb.Service_Container{
			MachineId: r.MachineID,
			Container: containerJSON,
		}
	}

	ctr := records[0].Container
	svc := &pb.Service{
		Id:         ctr.ServiceID(),
		Name:       ctr.ServiceName(),
		Mode:       ctr.ServiceMode(),
		Containers: containers,
	}
	return &pb.InspectServiceResponse{Service: svc}, nil
}
