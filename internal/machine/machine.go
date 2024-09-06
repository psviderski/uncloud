package machine

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strconv"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/cluster"
	"uncloud/internal/machine/network"
)

type Config struct {
	// DataDir is the directory where the machine stores its persistent state.
	DataDir     string
	APISockPath string
}

type Machine struct {
	config Config
	state  *State

	networkServer *grpc.Server
	clusterState  *cluster.State
	cluster       *cluster.Server
	// TODO: create localServer for unix socket.
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
		config:        *config,
		state:         state,
		networkServer: grpc.NewServer(),
	}

	clusterStatePath := cluster.StatePath(config.DataDir)
	clusterState := cluster.NewState(clusterStatePath)
	if err = clusterState.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load cluster state: %w", err)
		}
	} else {
		// Cluster state is successfully loaded, start the cluster server.
		m.cluster = cluster.NewServer(clusterState)
		pb.RegisterClusterServer(m.networkServer, m.cluster)
	}

	return m, nil
}

func (m *Machine) Run(ctx context.Context) error {
	// Use an errgroup to coordinate error handling and graceful shutdown of multiple machine components.
	errGroup, ctx := errgroup.WithContext(ctx)

	// Start the network only if it is configured.
	if m.state.Network.IsConfigured() {
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

		errGroup.Go(func() error {
			if err = wgnet.Run(ctx); err != nil {
				return fmt.Errorf("WireGuard network failed: %w", err)
			}
			return nil
		})
	} else {
		slog.Info("Waiting for network configuration to start WireGuard network.")
	}

	// Start the machine API server if the management IP is configured for it.
	if m.state.Network.ManagementIP != (netip.Addr{}) {
		apiAddr := net.JoinHostPort(m.state.Network.ManagementIP.String(), strconv.Itoa(APIPort))
		listener, err := net.Listen("tcp", apiAddr)
		if err != nil {
			return fmt.Errorf("listen API port: %w", err)
		}

		errGroup.Go(func() error {
			slog.Info("Starting API server.", "addr", apiAddr)
			if err = m.networkServer.Serve(listener); err != nil {
				return fmt.Errorf("API server failed: %w", err)
			}
			return nil
		})
	}

	// Shutdown goroutine.
	errGroup.Go(func() error {
		<-ctx.Done()
		slog.Info("Stopping API server.")
		// TODO: implement timeout for graceful shutdown.
		m.networkServer.GracefulStop()
		slog.Info("API server stopped.")
		return nil
	})

	return errGroup.Wait()
}

// InitCluster resets the local machine and initialises a new cluster with it.
// TODO: ideally, this should be an RPC call to the machine API to correctly handle the leave request and reconfiguration.
func (m *Machine) InitCluster(machineName string, netPrefix netip.Prefix, users []*pb.User) error {
	var err error
	if machineName == "" {
		machineName, err = cluster.NewRandomMachineName()
		if err != nil {
			return fmt.Errorf("generate machine name: %w", err)
		}
	}

	// TODO: a proper cluster leave mechanism and machine reset should be implemented later.
	//  For now assume the cluster server is not running.
	clusterStatePath := cluster.StatePath(m.config.DataDir)
	clusterState := cluster.NewState(clusterStatePath)
	if err = clusterState.Save(); err != nil {
		return fmt.Errorf("save cluster state: %w", err)
	}
	clusterServer := cluster.NewServer(clusterState)
	// TODO: register and start the cluster server when this becomes an RPC call.

	if err = clusterServer.SetNetwork(netPrefix); err != nil {
		return fmt.Errorf("set cluster network: %w", err)
	}

	// Use the public and all routable IPs as endpoints.
	ips, err := network.ListRoutableIPs()
	if err != nil {
		return fmt.Errorf("list routable addresses: %w", err)
	}
	publicIP, err := network.GetPublicIP()
	// Ignore the error if failed to get the public IP using API services.
	if err == nil {
		ips = append([]netip.Addr{publicIP}, ips...)
	}
	endpoints := make([]*pb.IPPort, len(ips))
	for i, addr := range ips {
		addrPort := netip.AddrPortFrom(addr, network.WireGuardPort)
		endpoints[i] = pb.NewIPPort(addrPort)
	}

	// Register the new machine in the cluster to populate the state and get its ID and subnet.
	// Public and private keys have already been initialised in the machine state when it was created.
	req := &pb.AddMachineRequest{
		Name: machineName,
		Network: &pb.NetworkConfig{
			Endpoints: endpoints,
			PublicKey: m.state.Network.PublicKey,
		},
	}
	resp, err := clusterServer.AddMachine(context.Background(), req)
	if err != nil {
		return fmt.Errorf("add machine to cluster: %w", err)
	}

	subnet, err := resp.Machine.Network.Subnet.ToPrefix()
	if err != nil {
		return err
	}
	manageIP, err := resp.Machine.Network.ManagementIp.ToAddr()
	if err != nil {
		return err
	}
	// Add users to the cluster and build peers config from them.
	peers := make([]network.PeerConfig, len(users))
	for i, u := range users {
		if err = clusterServer.AddUser(u); err != nil {
			return fmt.Errorf("add user to cluster: %w", err)
		}
		userManageIP, uErr := u.Network.ManagementIp.ToAddr()
		if uErr != nil {
			return uErr
		}
		peers[i] = network.PeerConfig{
			ManagementIP: userManageIP,
			PublicKey:    u.Network.PublicKey,
		}
	}

	// Update the machine state with the new cluster configuration.
	m.state.ID = resp.Machine.Id
	m.state.Name = resp.Machine.Name
	m.state.Network = &network.Config{
		Subnet:       subnet,
		ManagementIP: manageIP,
		PrivateKey:   m.state.Network.PrivateKey,
		PublicKey:    m.state.Network.PublicKey,
		Peers:        peers,
	}
	if err = m.state.Save(); err != nil {
		return fmt.Errorf("save machine state: %w", err)
	}

	fmt.Printf("Cluster initialised with machine %q\n", m.state.Name)
	return nil
}
