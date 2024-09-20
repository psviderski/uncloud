package machine

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/network"
)

const (
	APIPort           = 51000
	DockerNetworkName = "uncloud"
	DockerUserChain   = "DOCKER-USER"
)

type networkController struct {
	state         *State
	wgnet         *network.WireGuardNetwork
	server        *grpc.Server
	newMachinesCh <-chan *pb.MachineInfo
	// TODO: DNS server/resolver listening on the machine IP, e.g. 10.210.0.1:53. It can't listen on 127.0.X.X
	//  like resolved does because it needs to be reachable from both the host and the containers.
}

func newNetworkController(state *State, server *grpc.Server, newMachCh <-chan *pb.MachineInfo) (
	*networkController, error,
) {
	slog.Info("Starting WireGuard network.")
	wgnet, err := network.NewWireGuardNetwork()
	if err != nil {
		return nil, fmt.Errorf("create WireGuard network: %w", err)
	}

	return &networkController{
		state:         state,
		wgnet:         wgnet,
		server:        server,
		newMachinesCh: newMachCh,
	}, nil
}

func (nc *networkController) Run(ctx context.Context) error {
	if err := nc.wgnet.Configure(*nc.state.Network); err != nil {
		return fmt.Errorf("configure WireGuard network: %w", err)
	}
	slog.Info("WireGuard network configured.")

	errGroup, ctx := errgroup.WithContext(ctx)

	// Start the network API server. Assume the management IP can't be changed when the network is running.
	apiAddr := net.JoinHostPort(nc.state.Network.ManagementIP.String(), strconv.Itoa(APIPort))
	listener, err := net.Listen("tcp", apiAddr)
	if err != nil {
		return fmt.Errorf("listen API port: %w", err)
	}
	errGroup.Go(
		func() error {
			slog.Info("Starting network API server.", "addr", apiAddr)
			if err := nc.server.Serve(listener); err != nil {
				return fmt.Errorf("network API server failed: %w", err)
			}
			return nil
		},
	)

	// Setup Docker network and iptables rules in a goroutine because it may block until the Docker daemon is ready.
	errGroup.Go(
		func() error {
			if err := nc.setupDockerNetwork(ctx); err != nil {
				return fmt.Errorf("setup Docker network: %w", err)
			}
			slog.Info("Docker network configured.")
			return nil
		},
	)

	// Handle new machines added to the cluster. Handling new machines and endpoint changes should be done
	// in separate goroutines to avoid a deadlock when reconfiguring the network.
	errGroup.Go(
		func() error {
			if err := nc.handleNewMachines(ctx); err != nil {
				return fmt.Errorf("handle new machines: %w", err)
			}
			return nil
		},
	)

	// TODO: run another goroutine to watch WG network endpoint changes and update the state accordingly.
	//  Network updates in the state should not occur outside of this controller.

	errGroup.Go(
		func() error {
			if err = nc.wgnet.Run(ctx); err != nil {
				return fmt.Errorf("WireGuard network failed: %w", err)
			}
			return nil
		},
	)

	// Wait for the context to be done and stop the network API server.
	errGroup.Go(
		func() error {
			<-ctx.Done()
			slog.Info("Stopping network API server.")
			// TODO: implement timeout for graceful shutdown.
			nc.server.GracefulStop()
			slog.Info("Network API server stopped.")
			return nil
		},
	)

	return errGroup.Wait()
}

func (nc *networkController) handleNewMachines(ctx context.Context) error {
	for {
		select {
		case minfo := <-nc.newMachinesCh:
			slog.Info("Handling new machine added to the cluster.", "name", minfo.Name)

			// Skip the current machine.
			nc.state.mu.RLock()
			currentMachID := nc.state.ID
			nc.state.mu.RUnlock()
			if minfo.Id == currentMachID {
				continue
			}

			if err := minfo.Network.Validate(); err != nil {
				slog.Error("Invalid machine network configuration.", "err", err)
				continue
			}
			// Ignore errors as they are already validated.
			subnet, _ := minfo.Network.Subnet.ToPrefix()
			manageIP, _ := minfo.Network.ManagementIp.ToAddr()
			endpoints := make([]netip.AddrPort, len(minfo.Network.Endpoints))
			for i, ep := range minfo.Network.Endpoints {
				addrPort, _ := ep.ToAddrPort()
				endpoints[i] = addrPort
			}

			peer := network.PeerConfig{
				Subnet:       &subnet,
				ManagementIP: manageIP,
				AllEndpoints: endpoints,
				PublicKey:    minfo.Network.PublicKey,
			}
			if len(endpoints) > 0 {
				peer.Endpoint = &endpoints[0]
			}

			nc.state.mu.Lock()
			// TODO: deduplicate peers by public key, maybe implement addNetworkPeer method in the state.
			nc.state.Network.Peers = append(nc.state.Network.Peers, peer)
			err := nc.state.Save()
			nc.state.mu.Unlock()
			if err != nil {
				return fmt.Errorf("save machine state: %w", err)
			}

			if err := nc.wgnet.Configure(*nc.state.Network); err != nil {
				return fmt.Errorf("configure network with new peer: %w", err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// TODO: method to shutdown network when leaving a cluster. Regular context cancellation shouldn't bring it down.
