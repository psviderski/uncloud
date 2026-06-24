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
	"github.com/psviderski/uncloud/internal/machine/corromigrate"
	"github.com/psviderski/uncloud/internal/machine/corroservice"
	"github.com/psviderski/uncloud/internal/machine/dns"
	"github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/internal/machine/firewall"
	"github.com/psviderski/uncloud/internal/machine/metrics"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/unregistry"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// machineSyncInterval is how often the machine info is republished to the cluster store to recover from
// failed synchronous syncs.
const machineSyncInterval = 60 * time.Second

// clusterController is the main controller for the machine that is a cluster member. It manages components such as
// the WireGuard network, API server listening the WireGuard network, Corrosion service, Docker network and containers,
// and others.
type clusterController struct {
	// machine is the parent machine.
	machine *Machine
	state   *State
	store   *store.Store

	wgnet           *network.WireGuardNetwork
	endpointChanges <-chan network.EndpointChangeEvent

	server       *grpc.Server
	corroService corroservice.Service
	// corrosionDir is the disk path that holds the Corrosion config and data.
	// TODO: remove in 0.22 assuming all pre 0.20 clusters upgraded their pre-v1 Corrosion.
	corrosionDir  string
	dockerService *docker.Service
	dockerCtrl    *docker.Controller
	// dockerReady is signalled when Docker is configured and ready for containers.
	dockerReady chan<- struct{}

	// syncMachineTrigger requests to sync the machine info to the cluster store.
	syncMachineTrigger chan struct{}
	// clusterReady is signalled when the cluster controller has finished initializing all components.
	clusterReady    chan<- struct{}
	caddyconfigCtrl *caddyconfig.Controller

	// dnsServer is the embedded internal DNS server for the cluster listening on the machine IP.
	dnsServer   *dns.Server
	dnsResolver *dns.ClusterResolver
	// unregistry is the embedded container registry that uses the local Docker (containerd) image store as its backend.
	unregistry *unregistry.Registry

	metricsServer *metrics.Server

	// stopped is a channel that is closed when the controller is stopped.
	stopped chan struct{}
}

func newClusterController(
	machine *Machine,
	store *store.Store,
	server *grpc.Server,
	corroService corroservice.Service,
	corrosionDir string,
	dockerService *docker.Service,
	dockerReady chan<- struct{},
	clusterReady chan<- struct{},
	caddyfileCtrl *caddyconfig.Controller,
	dnsServer *dns.Server,
	dnsResolver *dns.ClusterResolver,
	unregistry *unregistry.Registry,
	metricsServer *metrics.Server,
) (*clusterController, error) {
	slog.Info("Starting WireGuard network.")
	wgnet, err := network.NewWireGuardNetwork()
	if err != nil {
		return nil, fmt.Errorf("create WireGuard network: %w", err)
	}
	endpointChanges := wgnet.WatchEndpoints()

	return &clusterController{
		machine:            machine,
		state:              machine.state,
		store:              store,
		wgnet:              wgnet,
		endpointChanges:    endpointChanges,
		server:             server,
		corroService:       corroService,
		corrosionDir:       corrosionDir,
		dockerService:      dockerService,
		dockerCtrl:         docker.NewController(machine.state.ID, dockerService, store),
		dockerReady:        dockerReady,
		syncMachineTrigger: make(chan struct{}, 1),
		clusterReady:       clusterReady,
		caddyconfigCtrl:    caddyfileCtrl,
		dnsServer:          dnsServer,
		dnsResolver:        dnsResolver,
		unregistry:         unregistry,
		metricsServer:      metricsServer,
		stopped:            make(chan struct{}),
	}, nil
}

func (cc *clusterController) Run(ctx context.Context) error {
	defer close(cc.stopped)

	if err := firewall.ConfigureIptablesChains(network.MachineIP(cc.state.Network.Subnet),
		cc.state.Network.EffectiveWireGuardPort()); err != nil {
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

	// Apply the seed to finish Corrosion migrations from 0.x to 2026.x.x (upstream v1.0.0) if applicable.
	if err := corromigrate.ApplySeedIfPresent(ctx, cc.corrosionDir, cc.store); err != nil {
		return fmt.Errorf("apply corrosion migration seed: %w", err)
	}

	errGroup, ctx := errgroup.WithContext(ctx)

	// Start the WireGuard control loop before waiting for store sync. This ensures endpoint rotation happens
	// while waiting, allowing Corrosion to connect to peers.
	errGroup.Go(func() error {
		if err := cc.wgnet.Run(ctx); err != nil {
			return fmt.Errorf("WireGuard network failed: %w", err)
		}
		return nil
	})

	// Watch for WireGuard peer endpoint changes and update the machine state accordingly.
	errGroup.Go(func() error {
		cc.handleEndpointChanges(ctx)
		return nil
	})

	// Start the network API server before waiting for the store sync so the machine is reachable on the mesh
	// during the sync and can serve requests that don't depend on the store.
	// Assume the management IP can't be changed when the network is running.
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

	// Wait for the store database to sync to the minimum version before starting store-dependent components.
	// This prevents issues with using partially replicated data when the machine just joined the cluster,
	// e.g., an empty machine list causing WireGuard peer misconfiguration.
	if err = cc.waitStoreSync(ctx); err != nil {
		return fmt.Errorf("wait initial cluster store sync: %w", err)
	}

	// Check if waitStoreSync exited because the context was cancelled. Return early in that case.
	if ctx.Err() != nil {
		cc.stopAPIServer()
		return errGroup.Wait()
	}

	errGroup.Go(func() error {
		slog.Info("Starting metrics server.")
		if err := cc.metricsServer.Run(ctx); err != nil {
			return fmt.Errorf("metrics server failed: %w", err)
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

	// Keep the machine info in the cluster store in sync with the actual machine state (the source of truth).
	errGroup.Go(func() error {
		return cc.runMachineSync(ctx)
	})

	// Synchronise Docker containers to the cluster store.
	errGroup.Go(func() error {
		slog.Info("Watching Docker containers and syncing them to cluster store.")
		return cc.syncDockerContainers(ctx)
	})

	// Handle machine changes in the cluster. Handling machine and endpoint changes should be done
	// in separate goroutines to avoid a deadlock when reconfiguring the network.
	errGroup.Go(func() error {
		if err := cc.handleMachineChanges(ctx); err != nil {
			return fmt.Errorf("handle new machines: %w", err)
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

	// Signal that the cluster controller has finished starting all components.
	close(cc.clusterReady)
	slog.Info("Cluster controller finished starting all components.")
	// Wait for the context to be done and stop all servers and controllers.
	<-ctx.Done()

	cc.stopAPIServer()

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

	// Wait for all controllers to finish. The Corrosion service shutdown is handled by the machine after stopping all
	// local API servers that may still serve requests depending on the store.
	return errGroup.Wait()
}

// stopAPIServer gracefully stops the network API server with a timeout.
func (cc *clusterController) stopAPIServer() {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	stopped := make(chan struct{})
	go func() {
		slog.Info("Stopping network API server.")
		cc.server.GracefulStop()
		close(stopped)
	}()

	select {
	case <-ctx.Done():
		slog.Warn("Network API server graceful stop timed out, forcing stop.", "timeout", timeout)
		cc.server.Stop()
	case <-stopped:
	}

	slog.Info("Network API server stopped.")
}

// ensureDockerNetwork ensures that the Docker network is configured and ready for containers.
func (cc *clusterController) ensureDockerNetwork(ctx context.Context) error {
	if err := cc.dockerCtrl.WaitDaemonReady(ctx); err != nil {
		return fmt.Errorf("wait for Docker daemon: %w", err)
	}

	if err := cc.dockerCtrl.EnsureUncloudNetwork(
		ctx,
		cc.state.Network.Subnet,
		cc.state.Network.EffectiveMTU(),
		cc.dnsServer.ListenAddr(),
	); err != nil {
		return fmt.Errorf("ensure Docker network: %w", err)
	}

	// Signal that Docker is ready for containers.
	close(cc.dockerReady)

	return nil
}

// handleEndpointChanges watches for WireGuard peer endpoint changes and persists them to the machine state.
func (cc *clusterController) handleEndpointChanges(ctx context.Context) {
	for {
		select {
		case e, ok := <-cc.endpointChanges:
			if !ok {
				// The channel was closed, stop watching for changes.
				cc.endpointChanges = nil
				return
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
			return
		}
	}
}

// waitStoreSync blocks until the local store version >= state.MinStoreVersion and any known gaps are synced.
// No-op when MinStoreVersion is empty. Clears state.MinStoreVersion when reached.
func (cc *clusterController) waitStoreSync(ctx context.Context) error {
	target := cc.state.MinStoreVersion
	if len(target) == 0 {
		return nil
	}

	slog.Info("Waiting for the initial cluster store sync.", "actors", len(target))

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	// Periodic warning to surface stuck NAT/connectivity issues without aborting.
	warnInterval := 5 * time.Minute
	warnTimer := time.NewTimer(warnInterval)
	defer warnTimer.Stop()

	var (
		lastLagging    int
		lastErrLogTime time.Time
	)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-warnTimer.C:
			local, err := cc.store.Version(ctx)
			if err == nil {
				slog.Error("Cluster store sync still pending. Check connectivity to peers.",
					"lagging_actors", laggingActors(local, target))
			} else {
				slog.Error("Cluster store sync still pending. Check connectivity to peers.", "err", err)
			}
			warnTimer.Reset(warnInterval)
		case <-ticker.C:
			local, err := cc.store.Version(ctx)
			if err != nil {
				// Throttle error logs to once every 5 seconds.
				if time.Since(lastErrLogTime) >= 5*time.Second {
					slog.Error("Failed to get the cluster store version, retrying.", "err", err)
					lastErrLogTime = time.Now()
				}
				continue
			}

			lagging := laggingActors(local, target)
			if len(lagging) == 0 {
				// Per-actor max doesn't imply contiguous apply: corrosion can buffer X:N before
				// X:N-1 arrives and track the gap separately. Wait for any remaining gaps to be synced.
				if err := cc.waitKnownMissingChanges(ctx); err != nil {
					return fmt.Errorf("wait for known missing changes: %w", err)
				}
				// If the context was cancelled mid-gap-fill, don't persist a "synced" state.
				if ctx.Err() != nil {
					return nil
				}

				// Clear MinStoreVersion so next restart doesn't wait for sync.
				cc.state.mu.Lock()
				cc.state.MinStoreVersion = nil
				err = cc.state.Save()
				cc.state.mu.Unlock()
				if err != nil {
					return fmt.Errorf("save machine state after the initial cluster store sync: %w", err)
				}

				slog.Info("Cluster store completed the initial sync.", "actors", len(target))
				return nil
			}

			if len(lagging) != lastLagging {
				slog.Info("Syncing cluster store.", "lagging_actors", lagging)
				lastLagging = len(lagging)
			}
		}
	}
}

// laggingActors returns target actors whose local version is below the required value, as [have, need].
func laggingActors(local, target map[string]int64) map[string][2]int64 {
	lagging := make(map[string][2]int64)
	for actor, need := range target {
		if have := local[actor]; have < need {
			lagging[actor] = [2]int64{have, need}
		}
	}
	return lagging
}

// waitKnownMissingChanges polls the store until all known missing changes have been synced.
func (cc *clusterController) waitKnownMissingChanges(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			changes, err := cc.store.KnownMissingChanges(ctx)
			if err != nil {
				return fmt.Errorf("query known missing changes from cluster store: %w", err)
			}

			if len(changes) == 0 {
				slog.Debug("All known missing changes have been synced to the cluster store.")
				return nil
			}

			slog.Debug("Waiting for known missing changes to be synced to the cluster store.", "remaining",
				len(changes))
		}
	}
}

// runMachineSync keeps this machine's info in the cluster store in sync with the local state and the Docker engine
// version. Call RequestMachineSync to trigger an immediate sync.
func (cc *clusterController) runMachineSync(ctx context.Context) error {
	// Backfill legacy local state from the cluster store once before the first sync.
	if err := cc.backfillMachineState(ctx); err != nil {
		return fmt.Errorf("backfill machine state: %w", err)
	}

	if err := cc.syncMachineInfo(ctx); err != nil {
		slog.Error("Failed to sync machine info to cluster store.", "err", err)
	}

	ticker := time.NewTicker(machineSyncInterval)
	defer ticker.Stop()
	dockerRestarted := cc.dockerService.WatchDaemonRestart(ctx)

	for {
		select {
		case <-ctx.Done():
		case <-ticker.C: // Scheduled periodic sync.
		case <-cc.syncMachineTrigger: // Immediate sync request.
		case <-dockerRestarted: // Docker daemon restarted -- engine version may have changed.
		}

		// A pending restart signal can race with context cancellation and win the select.
		if ctx.Err() != nil {
			return nil
		}

		if err := cc.syncMachineInfo(ctx); err != nil {
			slog.Error("Failed to sync machine info to cluster store.", "err", err)
		}
	}
}

// RequestMachineSync triggers an immediate sync of this machine's info to the cluster store.
// It never blocks and coalesces with any already pending sync.
func (cc *clusterController) RequestMachineSync() {
	select {
	case cc.syncMachineTrigger <- struct{}{}:
	default:
	}
}

// syncMachineInfo republishes this machine's info from local state to the cluster store, skipping the
// write if the info is unchanged since the last successful write.
func (cc *clusterController) syncMachineInfo(ctx context.Context) error {
	publishedInfo, err := cc.store.GetMachine(ctx, cc.state.ID)
	if err != nil && !errors.Is(err, store.ErrMachineNotFound) {
		return fmt.Errorf("get machine from store: %w", err)
	}

	info := cc.machine.Info(ctx)
	if publishedInfo != nil {
		// Info leaves the Docker engine version empty when the engine is unavailable. Keep the previously
		// published version in that case rather than overwriting it with an empty value.
		if info.DockerVersion == "" {
			info.DockerVersion = publishedInfo.DockerVersion
		}

		// Skip the write if nothing changed since the last successful sync.
		if proto.Equal(info, publishedInfo) {
			return nil
		}
	}

	if err = cc.store.UpdateMachine(ctx, info); err != nil {
		if !errors.Is(err, store.ErrMachineNotFound) {
			return fmt.Errorf("update machine in store: %w", err)
		}
		// The machine row is missing (not created yet or lost). Recreate it.
		if err = cc.store.CreateMachine(ctx, info); err != nil {
			return fmt.Errorf("create machine in store: %w", err)
		}
	}

	slog.Info("Synced machine info to cluster store.", "id", info.Id, "name", info.Name)
	return nil
}

// backfillMachineState backfills legacy local state that predates local ownership of endpoints/public IP
// (implemented in 0.20) from the cluster store.
// TODO: remove after releasing 0.22.
func (cc *clusterController) backfillMachineState(ctx context.Context) error {
	cc.state.mu.Lock()
	defer cc.state.mu.Unlock()

	if len(cc.state.Network.Endpoints) > 0 && cc.state.PublicIP.IsValid() {
		return nil
	}

	existing, err := cc.store.GetMachine(ctx, cc.state.ID)
	if err != nil {
		if errors.Is(err, store.ErrMachineNotFound) {
			// No existing row to backfill from. syncMachineInfo will create it.
			return nil
		}
		return fmt.Errorf("get machine from store: %w", err)
	}

	changed := false
	if len(cc.state.Network.Endpoints) == 0 {
		if endpoints := endpointsToAddrPorts(existing.Network.GetEndpoints()); len(endpoints) > 0 {
			cc.state.Network.Endpoints = endpoints
			changed = true
		}
	}
	if !cc.state.PublicIP.IsValid() && existing.PublicIp != nil {
		if ip, _ := existing.PublicIp.ToAddr(); ip.IsValid() {
			cc.state.PublicIP = ip
			changed = true
		}
	}
	if changed {
		if err = cc.state.Save(); err != nil {
			return fmt.Errorf("save backfilled machine state: %w", err)
		}
	}
	return nil
}

// syncDockerContainers watches local Docker containers and syncs them to the cluster store.
// TODO: move this to the Docker controller.
func (cc *clusterController) syncDockerContainers(ctx context.Context) error {
	// Supervise the watch-and-sync pipeline until the context is done.
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
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("watch and sync containers to cluster store: %w", err)
	}

	return nil
}

// handleMachineChanges subscribes to machine changes in the cluster and reconfigures the network peers accordingly
// when changes occur. It returns an error when the subscription fails.
func (cc *clusterController) handleMachineChanges(ctx context.Context) error {
	machines, changes, err := cc.store.SubscribeMachines(ctx)
	if err != nil {
		return fmt.Errorf("subscribe to machine changes: %w", err)
	}
	slog.Info("Subscribed to machine changes in the cluster to reconfigure network peers.")

	// Assume the initial store synchronization when this machine first joined the cluster has already been completed.
	// So the machines should not be empty. But we still have a safety check to not reconfigure with an empty list,
	// which would remove all peers and lock this machine out of the cluster.
	// A list containing only this machine is a valid state (e.g. all other machines were removed) and should still
	// trigger reconfiguration to drop any stale peers.
	if len(machines) > 0 {
		slog.Info("Reconfiguring network peers with the current machines.", "machines", len(machines))
		if err = cc.configurePeers(machines); err != nil {
			slog.Error("Failed to configure peers.", "err", err)
		}
	}

	// For simplicity, reconfigure all peers on any change. The subscription closes the changes channel both on context
	// cancellation and when the subscription fails, so this loop exits in both cases.
	for range changes {
		slog.Info("Cluster machines changed, reconfiguring network peers.")
		if machines, err = cc.store.ListMachines(ctx); err != nil {
			slog.Error("Failed to list machines.", "err", err)
			continue
		}
		// A safety check for the exceptional case when something bad happened with the store. Reconfiguring with an
		// empty list would remove all peers and lock this machine out of the cluster.
		// See https://github.com/psviderski/uncloud/issues/155.
		if len(machines) == 0 {
			slog.Debug("Skipping peer reconfiguration: machines list in store is empty.")
			continue
		}
		if err = cc.configurePeers(machines); err != nil {
			slog.Error("Failed to configure peers.", "err", err)
		}
	}

	// The changes channel was closed. It's a clean shutdown if the context was cancelled, otherwise the subscription
	// failed and we return an error to fail the controller.
	if ctx.Err() != nil {
		return nil
	}
	return fmt.Errorf("subscription to machine changes in cluster store failed")
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
