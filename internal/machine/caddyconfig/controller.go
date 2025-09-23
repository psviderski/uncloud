package caddyconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/psviderski/uncloud/internal/fs"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
)

const (
	CaddyServiceName = "caddy"
	CaddyGroup       = "uncloud"
	VerifyPath       = "/.uncloud-verify"
)

// Controller monitors container changes in the cluster store and generates a configuration file for Caddy reverse
// proxy. The generated configuration allows Caddy to route external traffic to service containers across the internal
// network.
type Controller struct {
	machineID     string
	caddyfilePath string
	generator     *CaddyfileGenerator
	client        *CaddyAdminClient
	store         *store.Store
	log           *slog.Logger
}

func NewController(machineID, configDir, adminSock string, store *store.Store) (*Controller, error) {
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return nil, fmt.Errorf("create directory for Caddy configuration '%s': %w", configDir, err)
	}
	if err := fs.Chown(configDir, "", CaddyGroup); err != nil {
		return nil, fmt.Errorf("change owner of directory for Caddy configuration '%s': %w", configDir, err)
	}

	log := slog.With("component", "caddy-controller")
	client := NewCaddyAdminClient(adminSock)
	generator := NewCaddyfileGenerator(machineID, client, log)

	return &Controller{
		machineID:     machineID,
		caddyfilePath: filepath.Join(configDir, "Caddyfile"),
		generator:     generator,
		client:        client,
		store:         store,
		log:           log,
	}, nil
}

func (c *Controller) Run(ctx context.Context) error {
	containers, changes, err := c.store.SubscribeContainers(ctx)
	if err != nil {
		return fmt.Errorf("subscribe to container changes: %w", err)
	}
	c.log.Info("Subscribed to container changes in the cluster to generate Caddy configuration.")

	containers = filterHealthyContainers(containers)
	c.generateAndLoadCaddyfile(ctx, containers)

	// TODO: left for backward compatibility, remove later.
	if err = c.generateJSONConfig(containers); err != nil {
		c.log.Error("Failed to generate Caddy JSON configuration to disk.", "err", err)
	}

	for {
		select {
		case _, ok := <-changes:
			if !ok {
				return fmt.Errorf("containers subscription failed")
			}
			c.log.Info("Cluster containers changed, updating Caddy configuration.")

			containers, err = c.store.ListContainers(ctx, store.ListOptions{})
			if err != nil {
				c.log.Error("Failed to list containers.", "err", err)
				continue
			}
			containers = filterHealthyContainers(containers)
			c.generateAndLoadCaddyfile(ctx, containers)

			// TODO: left for backward compatibility, remove later.
			if err = c.generateJSONConfig(containers); err != nil {
				c.log.Error("Failed to generate Caddy JSON configuration to disk.", "err", err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// filterHealthyContainers filters out containers that are not healthy.
// TODO: Filters out containers from this machine that are likely unavailable. The availability can be determined
// by the cluster membership state of the machine that the container is running on. Implement machine membership
// check using Corrossion Admin client.
func filterHealthyContainers(containers []store.ContainerRecord) []store.ContainerRecord {
	healthy := make([]store.ContainerRecord, 0, len(containers))
	for _, cr := range containers {
		if cr.Container.Healthy() {
			healthy = append(healthy, cr)
		}
	}
	return healthy
}

func (c *Controller) generateAndLoadCaddyfile(ctx context.Context, containers []store.ContainerRecord) {
	// Check if Caddy is available before attempting to generate and load config.
	caddyAvailable := c.client.IsAvailable(ctx)
	caddyfile, err := c.generator.Generate(ctx, containers, caddyAvailable)
	if err != nil {
		c.log.Error("Failed to generate Caddyfile configuration.", "err", err)
		return
	}

	if !caddyAvailable {
		// Caddy is not running so the generated Caddyfile should not include user-defined configs thus must be valid.
		// It's safe to write the config to disk so that when Caddy is deployed on this machine, it can pick it up.
		if err = c.writeCaddyfile(caddyfile); err != nil {
			c.log.Error("Failed to write Caddyfile to disk.", "err", err)
			return
		}
		c.log.Debug("Caddy is not running on this machine, skipping configuration load.", "path", c.caddyfilePath)
		return
	}

	// Caddy is available, try to load the config which may fail if the config is invalid. Generally, a config can
	// pass the adaptation/validation step but still fail to load, for example, if it references resources that are
	// not available.
	if err = c.client.Load(ctx, caddyfile); err != nil {
		c.log.Error("Failed to load new Caddy configuration into local Caddy instance.",
			"err", err, "path", c.caddyfilePath)
		// Don't write invalid config to disk.
		return
	}

	// Config loaded successfully, now write it to disk.
	if err = c.writeCaddyfile(caddyfile); err != nil {
		c.log.Error("Failed to write Caddyfile to disk after successful load.", "err", err)
		// Config is already loaded in Caddy, so this is not critical.
	}

	c.log.Info("New Caddy configuration loaded into local Caddy instance.", "path", c.caddyfilePath)
}

// writeCaddyfile writes the Caddyfile content to disk with proper permissions.
func (c *Controller) writeCaddyfile(caddyfile string) error {
	if err := os.WriteFile(c.caddyfilePath, []byte(caddyfile), 0o640); err != nil {
		return fmt.Errorf("write Caddyfile to file '%s': %w", c.caddyfilePath, err)
	}
	if err := fs.Chown(c.caddyfilePath, "", CaddyGroup); err != nil {
		return fmt.Errorf("change owner of Caddyfile '%s': %w", c.caddyfilePath, err)
	}
	return nil
}

func (c *Controller) generateJSONConfig(containers []store.ContainerRecord) error {
	serviceContainers := make([]api.ServiceContainer, len(containers))
	for i, cr := range containers {
		serviceContainers[i] = cr.Container
	}

	config, err := GenerateJSONConfig(serviceContainers, c.machineID)
	if err != nil {
		return err
	}

	configBytes, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal Caddy configuration: %w", err)
	}
	configPath := filepath.Join(filepath.Dir(c.caddyfilePath), "caddy.json")

	if err = os.WriteFile(configPath, configBytes, 0o640); err != nil {
		return fmt.Errorf("write Caddy configuration to file '%s': %w", configPath, err)
	}
	if err = fs.Chown(configPath, "", CaddyGroup); err != nil {
		return fmt.Errorf("change owner of Caddy configuration file '%s': %w", configPath, err)
	}

	return nil
}
