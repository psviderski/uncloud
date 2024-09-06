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

	clusterStatePath := cluster.StatePath(config.DataDir)
	clusterState := cluster.NewState(clusterStatePath)
	if err = clusterState.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load cluster state: %w", err)
		}
		slog.Info("Cluster state file not found, creating a new one.", "path", clusterStatePath)
		if err = clusterState.Save(); err != nil {
			return nil, fmt.Errorf("save cluster state: %w", err)
		}
	}
	clusterServer := cluster.NewServer(clusterState)

	networkServer := grpc.NewServer()
	pb.RegisterClusterServer(networkServer, clusterServer)

	return &Machine{
		config:        *config,
		state:         state,
		networkServer: networkServer,
		cluster:       clusterServer,
	}, nil
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
