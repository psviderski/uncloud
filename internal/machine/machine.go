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
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/user"
	"strconv"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/cluster"
	"uncloud/internal/machine/network"
)

const (
	DefaultAPISockPath  = "/run/uncloud.sock"
	DefaultAPISockGroup = "uncloud"
)

type Config struct {
	// DataDir is the directory where the machine stores its persistent state.
	DataDir     string
	APISockPath string
}

type Machine struct {
	pb.UnimplementedMachineServer

	config Config
	state  *State
	// initialised is closed when the machine is initialised as a member of a cluster.
	initialised chan struct{}

	localServer   *grpc.Server
	networkServer *grpc.Server

	clusterState *cluster.State
	cluster      *cluster.Server
}

func NewMachine(config *Config) (*Machine, error) {
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

	m := &Machine{
		config:      *config,
		state:       state,
		initialised: make(chan struct{}),

		localServer:   grpc.NewServer(),
		networkServer: grpc.NewServer(),
	}
	pb.RegisterMachineServer(m.localServer, m)
	pb.RegisterMachineServer(m.networkServer, m)

	clusterStatePath := cluster.StatePath(config.DataDir)
	clusterState := cluster.NewState(clusterStatePath)
	if err = clusterState.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load cluster state: %w", err)
		}
	} else {
		// Cluster state is successfully loaded, start the cluster server.
		m.cluster = cluster.NewServer(clusterState)
		pb.RegisterClusterServer(m.localServer, m.cluster)
		pb.RegisterClusterServer(m.networkServer, m.cluster)
	}

	if m.IsInitialised() {
		close(m.initialised)
	}

	return m, nil
}

// IsInitialised returns true if the machine has been configured as a member of a cluster,
// either by initialising a new cluster on it or joining an existing one.
func (m *Machine) IsInitialised() bool {
	return m.state.ID != ""
}

func (m *Machine) Run(ctx context.Context) error {
	// Use an errgroup to coordinate error handling and graceful shutdown of multiple machine components.
	errGroup, ctx := errgroup.WithContext(ctx)

	// Start the machine local API server.
	apiSockPath := DefaultAPISockPath
	if m.config.APISockPath != "" {
		apiSockPath = m.config.APISockPath
	}
	localListener, err := listenUnixSocket(apiSockPath)
	if err != nil {
		return fmt.Errorf("listen API unix socket %q: %w", apiSockPath, err)
	}
	errGroup.Go(
		func() error {
			slog.Info("Starting local API server.", "path", apiSockPath)
			if err := m.localServer.Serve(localListener); err != nil {
				return fmt.Errorf("local API server failed: %w", err)
			}
			return nil
		},
	)

	// Start the machine network API server if the management IP is configured for it.
	if m.state.Network.ManagementIP != (netip.Addr{}) {
		apiAddr := net.JoinHostPort(m.state.Network.ManagementIP.String(), strconv.Itoa(APIPort))
		networkListener, err := net.Listen("tcp", apiAddr)
		if err != nil {
			return fmt.Errorf("listen API port: %w", err)
		}

		errGroup.Go(
			func() error {
				slog.Info("Starting network API server.", "addr", apiAddr)
				if err = m.networkServer.Serve(networkListener); err != nil {
					return fmt.Errorf("network API server failed: %w", err)
				}
				return nil
			},
		)
	}

	// Start the WireGuard network after the machine is initialised as a member of a cluster.
	errGroup.Go(
		func() error {
			if !m.IsInitialised() {
				slog.Info(
					"Waiting for the machine to be initialised as a member of a cluster to start WireGuard network.",
				)
			}
			select {
			case <-m.initialised:
			case <-ctx.Done():
				return nil
			}

			slog.Info("Starting WireGuard network.")
			wgnet, err := network.NewWireGuardNetwork()
			if err != nil {
				return fmt.Errorf("create WireGuard network: %w", err)
			}
			if err = wgnet.Configure(*m.state.Network); err != nil {
				return fmt.Errorf("configure WireGuard network: %w", err)
			}

			//ctx, cancel := context.WithCancel(context.Background())
			//go wgnet.WatchEndpoints(ctx, peerEndpointChangeNotifier)

			//addrs, err := network.ListRoutableIPs()
			//if err != nil {
			//	return err
			//}
			//fmt.Println("Addresses:", addrs)

			if err = wgnet.Run(ctx); err != nil {
				return fmt.Errorf("WireGuard network failed: %w", err)
			}
			return nil
		},
	)

	// Shutdown goroutine.
	errGroup.Go(
		func() error {
			<-ctx.Done()
			slog.Info("Stopping network API server.")
			// TODO: implement timeout for graceful shutdown.
			m.networkServer.GracefulStop()
			slog.Info("network API server stopped.")

			slog.Info("Stopping local API server.")
			// TODO: implement timeout for graceful shutdown.
			m.localServer.GracefulStop()
			slog.Info("local API server stopped.")
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
	//  For now assume the cluster server is not running.
	clusterStatePath := cluster.StatePath(m.config.DataDir)
	clusterState := cluster.NewState(clusterStatePath)
	if err = clusterState.Save(); err != nil {
		return nil, status.Errorf(codes.Internal, "save cluster state: %v", err)
	}
	clusterServer := cluster.NewServer(clusterState)
	// TODO: register and start the cluster server.

	if err = clusterServer.SetNetwork(req.Network); err != nil {
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
	addResp, err := clusterServer.AddMachine(ctx, addReq)
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
		if err = clusterServer.AddUser(req.User); err != nil {
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
	slog.Info("Cluster initialised.", "machine", m.state.Name)
	// Signal that the machine is initialised as a member of a cluster.
	close(m.initialised)

	resp := &pb.InitClusterResponse{
		Machine: addResp.Machine,
	}
	return resp, nil
}
