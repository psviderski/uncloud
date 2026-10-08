package caddyconfig

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/psviderski/uncloud/internal/fs"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
)

const (
	CaddyServiceName = "caddy"
	CaddyGroup       = "uncloud"
	VerifyPath       = "/.uncloud-verify"
	// periodicReconciliationInterval is the interval at which the controller reconciles the Caddyfile
	// even if no container changes are observed.
	periodicReconciliationInterval = 30 * time.Second
)

// Controller keeps the local Caddy reverse proxy in sync with container state from the cluster store. It writes a
// Caddyfile that routes external traffic to healthy service containers across the internal network.
//
// Current Caddy deployments share one Caddyfile and one admin socket per machine. The controller must preserve a
// saved full Caddyfile when Caddy is unavailable because that file may be needed to restart or roll back a container.
// It writes a reduced bootstrap config only when no full file exists, including when an older offline file must be
// replaced. The shared layout cannot give overlapping Caddy revisions separate configurations.
type Controller struct {
	machineID     string
	caddyfilePath string
	service       *Service
	generator     *CaddyfileGenerator
	client        *CaddyAdminClient
	store         *store.Store
	log           *slog.Logger
	// Tests use their own group so they can exercise real file publication without the daemon's uncloud group.
	fileGroup string
	// lastFingerprint records the healthy application-container inputs used for the last full Caddyfile that was
	// successfully loaded into Caddy and saved to disk. It excludes the Caddy container, whose identity, start time,
	// and global config are tracked separately below.
	lastFingerprint []containerFingerprint
	lastCaddyCtrID  string
	lastStartedAt   string
	lastGlobal      string
	lastSavedBody   string
}

// containerFingerprint contains container identity and routing inputs that can change the generated Caddyfile.
// It excludes incidental Docker state so unrelated container updates do not cause a Caddy reload.
type containerFingerprint struct {
	ID          string
	IP          netip.Addr
	Ports       []api.PortSpec
	CaddyConfig string
}

// Equal returns whether two fingerprints describe the same container input to the Caddyfile generator.
func (f containerFingerprint) Equal(other containerFingerprint) bool {
	return f.ID == other.ID &&
		f.IP == other.IP &&
		api.PortsEqual(f.Ports, other.Ports) &&
		f.CaddyConfig == other.CaddyConfig
}

func NewController(machineID string, service *Service, adminSock string, store *store.Store) (*Controller, error) {
	configDir := service.configDir
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return nil, fmt.Errorf("create directory for Caddy configuration '%s': %w", configDir, err)
	}
	if err := fs.Chown(configDir, "", CaddyGroup); err != nil {
		return nil, fmt.Errorf("change owner of directory for Caddy configuration '%s': %w", configDir, err)
	}

	log := slog.With("component", "caddy-controller")
	client := NewCaddyAdminClient(adminSock)

	// The generator is initialised by Run() after resolving the machine name from the store.
	return &Controller{
		machineID:     machineID,
		caddyfilePath: filepath.Join(configDir, "Caddyfile"),
		service:       service,
		client:        client,
		store:         store,
		log:           log,
		fileGroup:     CaddyGroup,
	}, nil
}

// Run reconciles on container changes and every periodicReconciliationInterval. Failed work is retried every second,
// but repeated retry failures are logged only during the periodic check.
// The periodic check also covers missed events and outstanding failures after the daemon restarts.
func (c *Controller) Run(ctx context.Context) error {
	// Default the machine name to the machine ID so the Caddyfile header still carries a stable identifier if
	// the store lookup fails.
	machineName := c.machineID
	if m, err := c.store.GetMachine(ctx, c.machineID); err != nil {
		c.log.Error("Failed to get machine from store, Caddy configuration will use machine ID as the name.",
			"machine_id", c.machineID, "err", err)
	} else {
		machineName = m.Name
	}
	c.generator = NewCaddyfileGenerator(c.machineID, machineName, c.client, c.log)

	containers, changes, err := c.store.SubscribeContainers(ctx)
	if err != nil {
		return fmt.Errorf("subscribe to container changes: %w", err)
	}
	c.log.Info("Subscribed to container changes in the cluster to generate Caddy configuration.")

	var retryC <-chan time.Time
	periodic := time.NewTicker(periodicReconciliationInterval)
	defer periodic.Stop()

	handleResult := func(err error, logFailure bool) {
		c.service.setReconciliationResult(err)
		if err == nil {
			retryC = nil
			return
		}
		if logFailure {
			c.log.Error("Failed to reconcile Caddy configuration, will retry.", "err", err)
		}
		retryC = time.After(time.Second)
	}

	reconcile := func(logFailure bool) {
		ctrs, err := c.store.ListContainers(ctx, store.ListOptions{})
		if err != nil {
			err = fmt.Errorf("list containers: %w", err)
		} else {
			err = c.generateAndLoadCaddyfile(ctx, ctrs)
		}
		handleResult(err, logFailure)
	}

	// The subscription supplies an initial snapshot, so the first attempt need not wait for a change event.
	handleResult(c.generateAndLoadCaddyfile(ctx, containers), true)

	for {
		select {
		case _, ok := <-changes:
			if !ok {
				return fmt.Errorf("subscription to container changes in cluster store failed")
			}
			c.log.Debug("Cluster containers changed, regenerating Caddy configuration.")

			reconcile(true)
		case <-periodic.C:
			reconcile(true)
		case <-retryC:
			reconcile(false)
		case <-ctx.Done():
			return nil
		}
	}
}

// filterHealthyContainers filters out unhealthy, hook, and caddy containers.
// TODO: Filters out containers from this machine that are likely unavailable. The availability can be determined
// by the cluster membership state of the machine that the container is running on. Implement machine membership
// check using Corrossion Admin client.
func filterHealthyContainers(containers []store.ContainerRecord) []store.ContainerRecord {
	healthy := make([]store.ContainerRecord, 0, len(containers))
	for _, cr := range containers {
		if cr.Container.IsHook() {
			continue
		}
		if cr.Container.ServiceName() == CaddyServiceName {
			continue
		}
		if cr.Container.Healthy() {
			healthy = append(healthy, cr)
		}
	}
	return healthy
}

// generateAndLoadCaddyfile reconciles the shared Caddyfile for the selected local Caddy container. A full candidate
// reaches disk only after Caddy accepts it. Without an admin endpoint, the controller keeps a saved full file intact
// and writes a bootstrap config only when the file is absent or already a bootstrap.
func (c *Controller) generateAndLoadCaddyfile(ctx context.Context, containers []store.ContainerRecord) error {
	caddyCtr := selectLocalCaddyContainer(containers, c.machineID)
	if caddyCtr == nil {
		// Caddy is not running locally which means there is no reliable source for the global config, skipping.
		return nil
	}

	saved, readErr := os.ReadFile(c.caddyfilePath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read saved Caddyfile: %w", readErr)
	}
	haveSaved := readErr == nil

	healthyCtrs := filterHealthyContainers(containers)
	fingerprint := fingerprintContainers(healthyCtrs)

	if !caddyCtr.State.Running || !c.client.IsAvailable() {
		if haveSaved && !isBootstrapCaddyfile(string(saved)) {
			// Caddy may need this file to restart or roll back. A hosts-only replacement could drop
			// storage or other global settings while the admin API is unavailable.
			if caddyCtr.State.Running {
				return fmt.Errorf("caddy admin socket unavailable, preserving saved Caddyfile")
			}
			return nil
		}

		bootstrap, err := c.generator.Generate(ctx, *caddyCtr, healthyCtrs, true)
		if err != nil {
			return fmt.Errorf("generate Caddy bootstrap config: %w", err)
		}
		if err = c.writeCaddyfileIfChanged(bootstrap); err != nil {
			return fmt.Errorf("save Caddy bootstrap config: %w", err)
		}
		if caddyCtr.State.Running {
			return fmt.Errorf("caddy admin socket unavailable, bootstrap config saved")
		}
		return nil
	}

	if haveSaved &&
		caddyfileBody(string(saved)) == c.lastSavedBody &&
		!isBootstrapCaddyfile(string(saved)) &&
		c.lastCaddyCtrID == caddyCtr.ID &&
		c.lastStartedAt == caddyCtr.State.StartedAt &&
		c.lastGlobal == caddyCtr.ServiceSpec.CaddyConfig() &&
		slices.EqualFunc(fingerprint, c.lastFingerprint, containerFingerprint.Equal) {
		// No changes to the inputs that affect the generated Caddyfile, so no reload is needed.
		return nil
	}

	caddyfile, err := c.generator.Generate(ctx, *caddyCtr, healthyCtrs, false)
	if err != nil {
		return fmt.Errorf("generate Caddyfile: %w", err)
	}
	// Try to load the config which may fail if the config is invalid. Generally, a config can pass
	// the adaptation/validation step but still fail to load, for example, if it references resources
	// that are not available.
	if err = c.client.Load(ctx, caddyfile); err != nil {
		return fmt.Errorf("load Caddyfile: %w", err)
	}
	if err = c.writeCaddyfileIfChanged(caddyfile); err != nil {
		return fmt.Errorf("save loaded Caddyfile: %w", err)
	}
	c.lastFingerprint = fingerprint
	c.lastCaddyCtrID = caddyCtr.ID
	c.lastStartedAt = caddyCtr.State.StartedAt
	c.lastGlobal = caddyCtr.ServiceSpec.CaddyConfig()
	c.lastSavedBody = caddyfileBody(caddyfile)
	c.log.Info("New Caddy configuration loaded into local Caddy instance.", "path", c.caddyfilePath)

	return nil
}

// isBootstrapCaddyfile distinguishes a reduced startup file from a full file that must survive Caddy outage.
// It also recognizes the older daemon's offline file. That file omitted every x-caddy block, including globals,
// so treating it as a protected full file would preserve the configuration-loss bug:
// https://github.com/psviderski/uncloud/issues/412
func isBootstrapCaddyfile(caddyfile string) bool {
	return strings.Contains(caddyfile, bootstrapMarker) || strings.Contains(caddyfile, legacyBootstrapMarker)
}

// fingerprintContainers returns a fingerprint of containers that the Caddyfile generator depends on.
func fingerprintContainers(containers []store.ContainerRecord) []containerFingerprint {
	fingerprints := make([]containerFingerprint, len(containers))
	for i, cr := range containers {
		// Ignore ports parsing error as not much we can do about it. The generator just logs them and skips
		// the container.
		ports, _ := cr.Container.ServicePorts()
		fingerprints[i] = containerFingerprint{
			ID:          cr.Container.ID,
			IP:          cr.Container.UncloudNetworkIP(),
			Ports:       ports,
			CaddyConfig: cr.Container.ServiceSpec.CaddyConfig(),
		}
	}
	slices.SortFunc(fingerprints, func(a, b containerFingerprint) int {
		return strings.Compare(a.ID, b.ID)
	})

	return fingerprints
}

// selectLocalCaddyContainer chooses which local Caddy container supplies global Caddy config. A running container
// takes precedence over a newer stopped replacement, even if the running container is unhealthy. If two revisions
// run at once, the shared admin socket does not identify its owner, so this choice remains best effort.
func selectLocalCaddyContainer(records []store.ContainerRecord, machineID string) *api.ServiceContainer {
	var selected *api.ServiceContainer
	for _, cr := range records {
		ctr := cr.Container
		if cr.MachineID != machineID || ctr.ServiceName() != CaddyServiceName || ctr.IsHook() {
			continue
		}
		if selected == nil {
			selected = &ctr
			continue
		}
		if ctr.State.Running != selected.State.Running {
			if ctr.State.Running {
				selected = &ctr
			}
			continue
		}
		if ctr.CreatedTime().Compare(selected.CreatedTime()) > 0 {
			selected = &ctr
		}
	}

	return selected
}

// writeCaddyfileIfChanged atomically replaces the saved Caddyfile only if its body differs from the new content.
func (c *Controller) writeCaddyfileIfChanged(caddyfile string) error {
	saved, err := os.ReadFile(c.caddyfilePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read Caddyfile '%s': %w", c.caddyfilePath, err)
	}
	if err == nil && caddyfileBody(caddyfile) == caddyfileBody(string(saved)) {
		return nil
	}

	dir := filepath.Dir(c.caddyfilePath)
	tmp, err := os.CreateTemp(dir, ".Caddyfile-*")
	if err != nil {
		return fmt.Errorf("create temporary Caddyfile: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if err = tmp.Chmod(0o640); err != nil {
		return fmt.Errorf("set temporary Caddyfile permissions: %w", err)
	}
	if err = fs.Chown(tmp.Name(), "", c.fileGroup); err != nil {
		return fmt.Errorf("change owner of temporary Caddyfile: %w", err)
	}

	if _, err = tmp.WriteString(caddyfile); err != nil {
		return fmt.Errorf("write temporary Caddyfile: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary Caddyfile: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temporary Caddyfile: %w", err)
	}
	if err = os.Rename(tmp.Name(), c.caddyfilePath); err != nil {
		return fmt.Errorf("replace Caddyfile '%s': %w", c.caddyfilePath, err)
	}

	return nil
}

// caddyfileBody returns the Caddyfile content without its first line, which carries a generation timestamp that
// rotates on every regeneration.
func caddyfileBody(caddyfile string) string {
	if _, after, ok := strings.Cut(caddyfile, "\n"); ok {
		return after
	}
	return caddyfile
}
