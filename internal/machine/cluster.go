package machine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/caddyconfig"
	"github.com/psviderski/uncloud/internal/machine/constants"
	"github.com/psviderski/uncloud/internal/machine/corroservice"
	"github.com/psviderski/uncloud/internal/machine/dns"
	"github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/internal/machine/firewall"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/internal/migrate"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/unregistry"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

// clusterController is the main controller for the machine that is a cluster member. It manages components such as
// the WireGuard network, API server listening the WireGuard network, Corrosion service, Docker network and containers,
// and others.
type clusterController struct {
	state *State
	store *store.Store

	wgnet           *network.WireGuardNetwork
	endpointChanges <-chan network.EndpointChangeEvent

	server       *grpc.Server
	corroService corroservice.Service
	dockerCtrl   *docker.Controller
	// dockerReady is signalled when Docker is configured and ready for containers.
	dockerReady     chan<- struct{}
	caddyconfigCtrl *caddyconfig.Controller

	// dnsServer is the embedded internal DNS server for the cluster listening on the machine IP.
	dnsServer   *dns.Server
	dnsResolver *dns.ClusterResolver
	// unregistry is the embedded container registry that uses the local Docker (containerd) image store as its backend.
	unregistry *unregistry.Registry

	// stopped is a channel that is closed when the controller is stopped.
	stopped chan struct{}
}

func newClusterController(
	state *State,
	store *store.Store,
	server *grpc.Server,
	corroService corroservice.Service,
	dockerService *docker.Service,
	dockerReady chan<- struct{},
	caddyfileCtrl *caddyconfig.Controller,
	dnsServer *dns.Server,
	dnsResolver *dns.ClusterResolver,
	unregistry *unregistry.Registry,
) (*clusterController, error) {
	slog.Info("Starting WireGuard network.")
	wgnet, err := network.NewWireGuardNetwork()
	if err != nil {
		return nil, fmt.Errorf("create WireGuard network: %w", err)
	}
	endpointChanges := wgnet.WatchEndpoints()

	return &clusterController{
		state:           state,
		store:           store,
		wgnet:           wgnet,
		endpointChanges: endpointChanges,
		server:          server,
		corroService:    corroService,
		dockerCtrl:      docker.NewController(state.ID, dockerService, store),
		dockerReady:     dockerReady,
		caddyconfigCtrl: caddyfileCtrl,
		dnsServer:       dnsServer,
		dnsResolver:     dnsResolver,
		unregistry:      unregistry,
		stopped:         make(chan struct{}),
	}, nil
}

func (cc *clusterController) Run(ctx context.Context) error {
	defer close(cc.stopped)

	if err := firewall.ConfigureIptablesChains(network.MachineIP(cc.state.Network.Subnet)); err != nil {
		return fmt.Errorf("configure iptables chains: %w", err)
	}

	if err := cc.ensureDockerNetwork(ctx); err != nil {
		return err
	}
	slog.Info("Docker network configured.")

	if err := cc.wgnet.Configure(*cc.state.Network); err != nil {
		return fmt.Errorf("configure WireGuard network: %w", err)
	}
	slog.Info("WireGuard network configured.")

	if cc.corroService.Running() {
		// Corrosion service was running before the WireGuard network was configured so we need to restart it.
		slog.Info("Restarting corrosion service to apply new configuration with WireGuard network.")
		if err := cc.corroService.Restart(ctx); err != nil {
			return fmt.Errorf("restart corrosion service: %w", err)
		}
		slog.Info("Corrosion service restarted.")
	} else {
		slog.Info("Starting corrosion service.")
		if err := cc.corroService.Start(ctx); err != nil {
			return fmt.Errorf("start corrosion service: %w", err)
		}
		slog.Info("Corrosion service started.")
	}

	errGroup, ctx := errgroup.WithContext(ctx)

	// Start the network API server. Assume the management IP can't be changed when the network is running.
	apiAddr := net.JoinHostPort(cc.state.Network.ManagementIP.String(), strconv.Itoa(constants.MachineAPIPort))
	listener, err := net.Listen("tcp", apiAddr)
	if err != nil {
		return fmt.Errorf("listen API port: %w", err)
	}

	errGroup.Go(func() error {
		slog.Info("Starting network API server.", "addr", apiAddr)
		if err := cc.server.Serve(listener); err != nil {
			return fmt.Errorf("network API server failed: %w", err)
		}
		return nil
	})

	errGroup.Go(func() error {
		slog.Info("Starting embedded DNS resolver.")
		if err := cc.dnsResolver.Run(ctx); err != nil {
			return fmt.Errorf("embedded DNS resolver failed: %w", err)
		}
		return nil
	})

	// The Docker network must be created before starting the DNS server because it listens on the machine IP.
	errGroup.Go(func() error {
		slog.Info("Starting embedded DNS server.")
		if err := cc.dnsServer.Run(ctx); err != nil {
			return fmt.Errorf("embedded DNS server failed: %w", err)
		}
		return nil
	})

	// Synchronise Docker containers to the cluster store.
	errGroup.Go(func() error {
		slog.Info("Watching Docker containers and syncing them to cluster store.")
		return cc.syncDockerContainers(ctx)
	})

	// Check for Caddy namespace migration after Docker containers have been synced to the store.
	// This is a one-time migration for existing deployments from pre-namespace versions.
	errGroup.Go(func() error {
		// Wait a moment for the initial container sync to complete before checking for migration.
		time.Sleep(500 * time.Millisecond)

		if err := cc.migrateCaddyNamespace(ctx, apiAddr); err != nil {
			slog.Error("Failed to migrate Caddy to system namespace.", "err", err)
		}
		return nil
	})

	// Handle machine changes in the cluster. Handling machine and endpoint changes should be done
	// in separate goroutines to avoid a deadlock when reconfiguring the network.
	errGroup.Go(func() error {
		if err := cc.handleMachineChanges(ctx); err != nil {
			return fmt.Errorf("handle new machines: %w", err)
		}
		return nil
	})

	// Watch for WireGuard peer endpoint changes and update the machine state accordingly.
	errGroup.Go(func() error {
		for {
			select {
			case e, ok := <-cc.endpointChanges:
				if !ok {
					// The channel was closed, stop watching for changes.
					cc.endpointChanges = nil
					return nil
				}

				cc.state.mu.Lock()
				for i := range cc.state.Network.Peers {
					if cc.state.Network.Peers[i].PublicKey.Equal(e.PublicKey) {
						cc.state.Network.Peers[i].Endpoint = &e.Endpoint
						break
					}
				}
				if err := cc.state.Save(); err != nil {
					slog.Error("Failed to save machine state.", "err", err)
				}
				cc.state.mu.Unlock()

				slog.Debug("Preserved endpoint change in the machine state.",
					"public_key", e.PublicKey, "endpoint", e.Endpoint)
			case <-ctx.Done():
				return nil
			}
		}
	})

	errGroup.Go(func() error {
		if err := cc.wgnet.Run(ctx); err != nil {
			return fmt.Errorf("WireGuard network failed: %w", err)
		}
		return nil
	})

	errGroup.Go(func() error {
		slog.Info("Starting caddyconfig controller.")
		if err := cc.caddyconfigCtrl.Run(ctx); err != nil {
			return fmt.Errorf("caddyconfig controller failed: %w", err)
		}
		return nil
	})

	if cc.unregistry != nil {
		errGroup.Go(func() error {
			slog.Info("Starting unregistry server.")
			if err := cc.unregistry.ListenAndServe(); err != nil {
				return fmt.Errorf("unregistry server failed: %w", err)
			}
			return nil
		})
	}

	// Wait for the context to be done and stop the network API server.
	<-ctx.Done()
	slog.Info("Stopping network API server.")
	// TODO: implement timeout for graceful shutdown.
	cc.server.GracefulStop()
	slog.Info("Network API server stopped.")

	// Stop the unregistry server with a timeout if it was started.
	if cc.unregistry != nil {
		unregTimeout := 30 * time.Second
		slog.Info("Stopping unregistry server.", "timeout", unregTimeout)
		unregCtx, cancel := context.WithTimeout(context.Background(), unregTimeout)
		defer cancel()

		if err = cc.unregistry.Shutdown(unregCtx); err != nil {
			return fmt.Errorf("unregistry server forced to shutdown: %w", err)
		}
		slog.Info("Unregistry server stopped.")
	}

	// Wait for all controllers to finish.
	err = errGroup.Wait()

	// It's safe to stop the Corrosion service after the controllers depending on it and API server are stopped.
	// Use a new context with a timeout as the current context is already canceled.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if corroErr := cc.corroService.Stop(ctx); corroErr != nil {
		err = errors.Join(err, fmt.Errorf("stop corrosion service: %w", corroErr))
	} else {
		slog.Info("Corrosion service stopped.")
	}

	return err
}

// ensureDockerNetwork ensures that the Docker network is configured and ready for containers.
func (cc *clusterController) ensureDockerNetwork(ctx context.Context) error {
	if err := cc.dockerCtrl.WaitDaemonReady(ctx); err != nil {
		return fmt.Errorf("wait for Docker daemon: %w", err)
	}

	if err := cc.dockerCtrl.EnsureUncloudNetwork(
		ctx,
		cc.state.Network.Subnet,
		cc.dnsServer.ListenAddr(),
	); err != nil {
		return fmt.Errorf("ensure Docker network: %w", err)
	}

	// Signal that Docker is ready for containers.
	close(cc.dockerReady)

	return nil
}

// syncDockerContainers watches local Docker containers and syncs them to the cluster store.
// TODO: move this to the Docker controller.
func (cc *clusterController) syncDockerContainers(ctx context.Context) error {
	// Retry to watch and sync containers until the context is done.
	boff := backoff.WithContext(backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(100*time.Millisecond),
		backoff.WithMaxInterval(5*time.Second),
		backoff.WithMaxElapsedTime(0),
	), ctx)
	watchAndSync := func() error {
		if wErr := cc.dockerCtrl.WatchAndSyncContainers(ctx); wErr != nil {
			slog.Error("Failed to watch and sync containers to cluster store, retrying.", "err", wErr)
			return wErr
		}
		return nil
	}
	if err := backoff.Retry(watchAndSync, boff); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return fmt.Errorf("watch and sync containers to cluster store: %w", err)
	}

	return nil
}

// handleMachineChanges subscribes to machine changes in the cluster and reconfigures the network peers accordingly
// when changes occur.
func (cc *clusterController) handleMachineChanges(ctx context.Context) error {
	for {
		// Retry to subscribe to machine changes indefinitely until the context is done.
		boff := backoff.WithContext(backoff.NewExponentialBackOff(
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
			if machines, changes, err = cc.store.SubscribeMachines(ctx); err != nil {
				slog.Info("Failed to subscribe to machine changes, retrying.", "err", err)
			}
			return err
		}
		if err = backoff.Retry(subscribe, boff); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			slog.Error("Unexpected error while retrying to subscribe to machine changes.", "err", err)
			continue
		}
		slog.Info("Subscribed to machine changes in the cluster to reconfigure network peers.")

		// The machine store may be empty when a machine first joins the cluster, before store synchronization
		// completes. Skip configuration now and apply it when the store changes are received.
		if len(machines) > 0 {
			slog.Info("Reconfiguring network peers with the current machines.", "machines", len(machines))
			if err = cc.configurePeers(machines); err != nil {
				slog.Error("Failed to configure peers.", "err", err)
			}
		}
		// For simplicity, reconfigure all peers on any change.
		for {
			select {
			// TODO: test when Corrosion fails and the subscription fails to resubscribe (after 1 minute). It seems
			//  the changes channel will be closed and this will become a busy loop. Perhaps, the outer for loop should
			//  be reworked as well.
			case <-changes:
				slog.Info("Cluster machines changed, reconfiguring network peers.")
				if machines, err = cc.store.ListMachines(ctx); err != nil {
					slog.Error("Failed to list machines.", "err", err)
					continue
				}
				// Skip reconfiguration if the machines list is empty. This can happen when joining the cluster.
				// Corrosion can notifies about table changes before the data is fully replicated.
				// Reconfiguring with an empty list would remove all peers and lock this machine out of the cluster.
				// See https://github.com/psviderski/uncloud/issues/155.
				if len(machines) == 0 {
					slog.Debug("Skipping peer reconfiguration: machines list in store is empty.")
					continue
				}
				if err = cc.configurePeers(machines); err != nil {
					slog.Error("Failed to configure peers.", "err", err)
				}
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func (cc *clusterController) configurePeers(machines []*pb.MachineInfo) error {
	if len(machines) == 0 {
		return fmt.Errorf("no machines to configure peers")
	}

	cc.state.mu.RLock()
	currentPeerEndpoints := make(map[string]*netip.AddrPort, len(cc.state.Network.Peers))
	for _, p := range cc.state.Network.Peers {
		currentPeerEndpoints[p.PublicKey.String()] = p.Endpoint
	}
	cc.state.mu.RUnlock()

	// Construct the list of peers from the machine configurations ensuring that the current endpoint is preserved.
	peers := make([]network.PeerConfig, 0, len(machines)-1)
	for _, m := range machines {
		// Skip the current machine.
		if m.Id == cc.state.ID {
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
	cc.state.mu.Lock()
	cc.state.Network.Peers = peers
	err := cc.state.Save()
	cc.state.mu.Unlock()
	if err != nil {
		return fmt.Errorf("save machine state: %w", err)
	}

	cc.state.mu.RLock()
	defer cc.state.mu.RUnlock()
	if err = cc.wgnet.Configure(*cc.state.Network); err != nil {
		return fmt.Errorf("configure network peers: %w", err)
	}
	return nil
}

// migrateCaddyNamespace checks if Caddy exists only in the default namespace and automatically migrates it.
// This is a one-time migration for existing deployments from pre-namespace versions.
func (cc *clusterController) migrateCaddyNamespace(ctx context.Context, apiAddr string) error {
	// List all containers named "caddy"
	opts := store.ListOptions{
		ServiceIDOrName: store.ServiceIDOrNameOptions{Name: "caddy"},
	}
	containers, err := cc.store.ListContainers(ctx, opts)
	if err != nil {
		return fmt.Errorf("check for Caddy service: %w", err)
	}

	if len(containers) == 0 {
		// No Caddy service found
		return nil
	}

	// Check which namespaces Caddy containers are in
	hasSystemNamespace := false
	hasDefaultNamespace := false

	for _, ctr := range containers {
		ns := ctr.Container.Config.Labels[api.LabelNamespace]
		if ns == "" {
			ns = api.DefaultNamespace
		}
		switch ns {
		case api.SystemNamespace:
			hasSystemNamespace = true
		case api.DefaultNamespace:
			hasDefaultNamespace = true
		}
	}

	// Only migrate if Caddy exists in default but not in system namespace
	// This ensures we don't interfere with user's custom caddy services
	if hasDefaultNamespace && !hasSystemNamespace {
		slog.Info("Detected Caddy service in default namespace from pre-namespace deployment. " +
			"Starting automatic migration to system namespace.")

		// Execute the migration by deploying Caddy to the system namespace
		if err := cc.executeCaddyMigration(ctx, apiAddr); err != nil {
			return fmt.Errorf("execute Caddy migration: %w", err)
		}

		slog.Info("Successfully migrated Caddy to system namespace.")
	}

	return nil
}

// executeCaddyMigration performs the actual Caddy migration by deploying it to the system namespace.
// This will create new containers with the correct namespace label and remove the old ones via rolling update.
func (cc *clusterController) executeCaddyMigration(ctx context.Context, apiAddr string) error {
	return migrate.MigrateCaddyToSystemNamespace(ctx, apiAddr)
}

// Cleanup cleans up the cluster resources such as the WireGuard network, iptables rules, Docker network and containers.
func (cc *clusterController) Cleanup() error {
	// Wait for the controller to stop before cleaning up.
	<-cc.stopped

	var errs []error
	if err := cc.dockerCtrl.Cleanup(); err != nil {
		errs = append(errs, fmt.Errorf("cleanup Docker resources: %w", err))
	}
	if err := cc.wgnet.Cleanup(); err != nil {
		errs = append(errs, fmt.Errorf("cleanup WireGuard network: %w", err))
	}
	if err := firewall.CleanupIptablesChains(); err != nil {
		errs = append(errs, fmt.Errorf("cleanup iptables chains: %w", err))
	}

	return errors.Join(errs...)
}
