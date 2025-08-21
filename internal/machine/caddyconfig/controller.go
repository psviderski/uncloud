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
	machineID string
	configDir string
	generator *CaddyfileGenerator
	store     *store.Store
	log       *slog.Logger
}

func NewController(machineID, configDir, adminSock string, store *store.Store) (*Controller, error) {
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return nil, fmt.Errorf("create directory for Caddy configuration '%s': %w", configDir, err)
	}
	if err := fs.Chown(configDir, "", CaddyGroup); err != nil {
		return nil, fmt.Errorf("change owner of directory for Caddy configuration '%s': %w", configDir, err)
	}

	log := slog.With("component", "caddy-controller")
	validator := NewCaddyAdminValidator(adminSock)
	generator := NewCaddyfileGenerator(machineID, validator, log)

	return &Controller{
		machineID: machineID,
		configDir: configDir,
		generator: generator,
		store:     store,
		log:       log,
	}, nil
}

func (c *Controller) Run(ctx context.Context) error {
	containers, changes, err := c.store.SubscribeContainers(ctx)
	if err != nil {
		return fmt.Errorf("subscribe to container changes: %w", err)
	}
	c.log.Info("Subscribed to container changes in the cluster to generate Caddy configuration.")

	containers = filterHealthyContainers(containers)
	if err = c.generateCaddyfile(ctx, containers); err != nil {
		return fmt.Errorf("generate Caddyfile configuration: %w", err)
	}
	if err = c.generateJSONConfig(containers); err != nil {
		return fmt.Errorf("generate Caddy JSON configuration: %w", err)
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

			if err = c.generateCaddyfile(ctx, containers); err != nil {
				c.log.Error("Failed to generate Caddyfile configuration.", "err", err)
			}
			if err = c.generateJSONConfig(containers); err != nil {
				c.log.Error("Failed to generate Caddy JSON configuration.", "err", err)
			}

			c.log.Info("Updated Caddy configuration.", "dir", c.configDir)
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

func (c *Controller) generateCaddyfile(ctx context.Context, containers []store.ContainerRecord) error {
	caddyfile, err := c.generator.Generate(ctx, containers)
	if err != nil {
		return fmt.Errorf("generate Caddyfile: %w", err)
	}
	caddyfilePath := filepath.Join(c.configDir, "Caddyfile")

	// TODO: use atomic file write to avoid partial loads on Caddy watch reload.
	if err = os.WriteFile(caddyfilePath, []byte(caddyfile), 0o640); err != nil {
		return fmt.Errorf("write Caddyfile to file '%s': %w", caddyfilePath, err)
	}
	if err = fs.Chown(caddyfilePath, "", CaddyGroup); err != nil {
		return fmt.Errorf("change owner of Caddyfile '%s': %w", caddyfilePath, err)
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
	configPath := filepath.Join(c.configDir, "caddy.json")

	if err = os.WriteFile(configPath, configBytes, 0o640); err != nil {
		return fmt.Errorf("write Caddy configuration to file '%s': %w", configPath, err)
	}
	if err = fs.Chown(configPath, "", CaddyGroup); err != nil {
		return fmt.Errorf("change owner of Caddy configuration file '%s': %w", configPath, err)
	}

	return nil
}
