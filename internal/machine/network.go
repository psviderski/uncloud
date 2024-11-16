package machine

import (
	"context"
	"errors"
	"fmt"
	"github.com/cenkalti/backoff/v4"
	"github.com/docker/docker/client"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"time"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/corroservice"
	"uncloud/internal/machine/network"
	"uncloud/internal/machine/store"
)

const (
	APIPort = 51000
)

type networkController struct {
	state *State
	store *store.Store

	wgnet           *network.WireGuardNetwork
	endpointChanges <-chan network.EndpointChangeEvent

	server       *grpc.Server
	corroService corroservice.Service

	// TODO: DNS server/resolver listening on the machine IP, e.g. 10.210.0.1:53. It can't listen on 127.0.X.X
	//  like resolved does because it needs to be reachable from both the host and the containers.
}

func newNetworkController(
	state *State, store *store.Store, server *grpc.Server, corroService corroservice.Service,
) (
	*networkController, error,
) {
	slog.Info("Starting WireGuard network.")
	wgnet, err := network.NewWireGuardNetwork()
	if err != nil {
		return nil, fmt.Errorf("create WireGuard network: %w", err)
	}
	endpointChanges := wgnet.WatchEndpoints()

	return &networkController{
		state:           state,
		store:           store,
		wgnet:           wgnet,
		endpointChanges: endpointChanges,
		server:          server,
		corroService:    corroService,
	}, nil
}

func (nc *networkController) Run(ctx context.Context) error {
	if err := nc.wgnet.Configure(*nc.state.Network); err != nil {
		return fmt.Errorf("configure WireGuard network: %w", err)
	}
	slog.Info("WireGuard network configured.")

	if nc.corroService.Running() {
		// Corrosion service was running before the WireGuard network was configured so we need to restart it.
		if err := nc.corroService.Restart(ctx); err != nil {
			return fmt.Errorf("restart corrosion service: %w", err)
		}
	} else {
		if err := nc.corroService.Start(ctx); err != nil {
			return fmt.Errorf("start corrosion service: %w", err)
		}
	}
	// TODO: Figure out if we need to manually stop the corrosion service when the context is done or just
	//  rely on systemd to handle service dependencies on its own.

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
			cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
			if err != nil {
				return fmt.Errorf("init Docker client: %w", err)
			}
			defer cli.Close()

			docker := NewDockerManager(cli, nc.store)
			if err := docker.WaitDaemonReady(ctx); err != nil {
				return fmt.Errorf("wait for Docker daemon: %w", err)
			}

			if err := docker.EnsureUncloudNetwork(ctx, nc.state.Network.Subnet); err != nil {
				return err
			}
			slog.Info("Docker network configured.")

			//if err := d.WatchAndSyncContainers(ctx); err != nil {
			//	return fmt.Errorf("watch and sync containers to store: %w", err)
			//}
			return nil
		},
	)

	// Handle machine changes in the cluster. Handling machine and endpoint changes should be done
	// in separate goroutines to avoid a deadlock when reconfiguring the network.
	errGroup.Go(
		func() error {
			if err := nc.handleMachineChanges(ctx); err != nil {
				return fmt.Errorf("handle new machines: %w", err)
			}
			return nil
		},
	)

	// Watch for endpoint changes and update the machine state accordingly.
	errGroup.Go(
		func() error {
			for {
				select {
				case e, ok := <-nc.endpointChanges:
					if !ok {
						// The channel was closed, stop watching for changes.
						nc.endpointChanges = nil
						return nil
					}

					nc.state.mu.Lock()
					for i := range nc.state.Network.Peers {
						if nc.state.Network.Peers[i].PublicKey.Equal(e.PublicKey) {
							nc.state.Network.Peers[i].Endpoint = &e.Endpoint
							break
						}
					}
					if err = nc.state.Save(); err != nil {
						slog.Error("Failed to save machine state.", "err", err)
					}
					nc.state.mu.Unlock()

					slog.Debug("Preserved endpoint change in the machine state.",
						"public_key", e.PublicKey, "endpoint", e.Endpoint)
				case <-ctx.Done():
					return nil
				}
			}
		},
	)

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

// handleMachineChanges subscribes to machine changes in the cluster and reconfigures the network peers accordingly.
func (nc *networkController) handleMachineChanges(ctx context.Context) error {
	for {
		// Retry to subscribe to machine changes indefinitely until the context is done.
		b := backoff.WithContext(backoff.NewExponentialBackOff(
			backoff.WithInitialInterval(1*time.Second),
			backoff.WithMaxInterval(60*time.Second),
			backoff.WithMaxElapsedTime(0),
		), ctx)

		var (
			machines []*pb.MachineInfo
			changes  <-chan struct{}
			err      error
		)
		subscribe := func() error {
			if machines, changes, err = nc.store.SubscribeMachines(ctx); err != nil {
				slog.Info("Failed to subscribe to machine changes, retrying.", "err", err)
			}
			return err
		}
		if err = backoff.Retry(subscribe, b); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			slog.Error("Unexpected error while retrying to subscribe to machine changes.", "err", err)
			continue
		}
		slog.Info("Subscribed to machine changes in the cluster to reconfigure network peers.")

		slog.Info("Reconfiguring network peers with the current machines.", "machines", len(machines))
		if err = nc.configurePeers(machines); err != nil {
			slog.Error("Failed to configure peers.", "err", err)
		}
		// For simplicity, reconfigure all peers on any change.
		for {
			select {
			case <-changes:
				slog.Info("Cluster machines changed, reconfiguring network peers.")
				if machines, err = nc.store.ListMachines(ctx); err != nil {
					slog.Error("Failed to list machines.", "err", err)
					continue
				}
				if err = nc.configurePeers(machines); err != nil {
					slog.Error("Failed to configure peers.", "err", err)
				}
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func (nc *networkController) configurePeers(machines []*pb.MachineInfo) error {
	nc.state.mu.RLock()
	currentPeerEndpoints := make(map[string]*netip.AddrPort, len(nc.state.Network.Peers))
	for _, p := range nc.state.Network.Peers {
		currentPeerEndpoints[p.PublicKey.String()] = p.Endpoint
	}
	nc.state.mu.RUnlock()

	// Construct the list of peers from the machine configurations ensuring that the current endpoint is preserved.
	peers := make([]network.PeerConfig, 0, len(machines)-1)
	for _, m := range machines {
		// Skip the current machine.
		if m.Id == nc.state.ID {
			continue
		}
		if err := m.Network.Validate(); err != nil {
			slog.Error("Invalid machine network configuration.", "machine", m.Name, "err", err)
			continue
		}
		// Ignore errors as they are already validated.
		subnet, _ := m.Network.Subnet.ToPrefix()
		manageIP, _ := m.Network.ManagementIp.ToAddr()
		endpoints := make([]netip.AddrPort, len(m.Network.Endpoints))
		for i, ep := range m.Network.Endpoints {
			addrPort, _ := ep.ToAddrPort()
			endpoints[i] = addrPort
		}
		peer := network.PeerConfig{
			Subnet:       &subnet,
			ManagementIP: manageIP,
			AllEndpoints: endpoints,
			PublicKey:    m.Network.PublicKey,
		}

		currentEndpoint := currentPeerEndpoints[peer.PublicKey.String()]
		if currentEndpoint != nil && slices.Contains(endpoints, *currentEndpoint) {
			peer.Endpoint = currentEndpoint
		} else if len(endpoints) > 0 {
			peer.Endpoint = &endpoints[0]
		}

		peers = append(peers, peer)
	}

	// Preserve the new list of peers in the machine state.
	nc.state.mu.Lock()
	nc.state.Network.Peers = peers
	err := nc.state.Save()
	nc.state.mu.Unlock()
	if err != nil {
		return fmt.Errorf("save machine state: %w", err)
	}

	nc.state.mu.RLock()
	defer nc.state.mu.RUnlock()
	if err = nc.wgnet.Configure(*nc.state.Network); err != nil {
		return fmt.Errorf("configure network peers: %w", err)
	}
	return nil
}

// TODO: method to shutdown network when leaving a cluster. Regular context cancellation shouldn't bring it down.
