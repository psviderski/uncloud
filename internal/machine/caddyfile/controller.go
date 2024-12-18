package caddyfile

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"log/slog"
	"os"
	"path/filepath"
	"uncloud/internal/api"
	"uncloud/internal/fs"
	"uncloud/internal/machine/store"
)

const CaddyGroup = "uncloud"

// Controller monitors container changes in the cluster store and generates a configuration file for Caddy reverse
// proxy. The generated Caddyfile allows Caddy to route external traffic to service containers across the internal
// network.
type Controller struct {
	store *store.Store
	path  string
}

func NewController(store *store.Store, path string) (*Controller, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create parent directory for Caddy configuration '%s': %w", dir, err)
	}
	if err := fs.Chown(dir, "", CaddyGroup); err != nil {
		return nil, fmt.Errorf("change owner of parent directory for Caddy configuration '%s': %w", dir, err)
	}

	return &Controller{
		store: store,
		path:  path,
	}, nil
}

func (c *Controller) Run(ctx context.Context) error {
	containerRecords, changes, err := c.store.SubscribeContainers(ctx)
	if err != nil {
		return fmt.Errorf("subscribe to container changes: %w", err)
	}
	slog.Info("Subscribed to container changes in the cluster to generate Caddy configuration.")

	containers, err := c.filterAvailableContainers(containerRecords)
	if err != nil {
		return fmt.Errorf("filter available containers: %w", err)
	}
	if err = c.generateConfig(containers); err != nil {
		return fmt.Errorf("generate Caddy configuration: %w", err)
	}

	for {
		select {
		case _, ok := <-changes:
			if !ok {
				return fmt.Errorf("containers subscription failed")
			}
			slog.Debug("Cluster containers changed, updating Caddy configuration.")

			containerRecords, err = c.store.ListContainers(ctx, store.ListOptions{})
			if err != nil {
				slog.Error("Failed to list containers.", "err", err)
				continue
			}
			containers, err = c.filterAvailableContainers(containerRecords)
			if err != nil {
				slog.Error("Failed to filter available containers.", "err", err)
				continue
			}
			if err = c.generateConfig(containers); err != nil {
				slog.Error("Failed to generate Caddy configuration.", "err", err)
			}

			slog.Debug("Updated Caddy configuration.", "path", c.path)
		case <-ctx.Done():
			return nil
		}
	}
}

// filterAvailableContainers filters out containers that are likely unavailable from this machine. The availability
// is determined by the cluster membership state of the machine that the container is running on.
// TODO: implement machine membership check using Corrossion Admin client.
func (c *Controller) filterAvailableContainers(containerRecords []*store.ContainerRecord) ([]*api.Container, error) {
	containers := make([]*api.Container, len(containerRecords))
	for i, cr := range containerRecords {
		containers[i] = cr.Container
	}
	return containers, nil
}

func (c *Controller) generateConfig(containers []*api.Container) error {
	servers := make(map[string]*caddyhttp.Server)
	servers["http"] = &caddyhttp.Server{
		Listen: []string{fmt.Sprintf(":%d", caddyhttp.DefaultHTTPPort)},
	}
	servers["https"] = &caddyhttp.Server{
		Listen: []string{fmt.Sprintf(":%d", caddyhttp.DefaultHTTPSPort)},
	}

	httpApp := caddyhttp.App{
		Servers: servers,
	}

	config := &caddy.Config{
		AppsRaw: make(caddy.ModuleMap),
	}
	var warnings []caddyconfig.Warning
	config.AppsRaw["http"] = caddyconfig.JSON(httpApp, &warnings)

	if len(warnings) > 0 {
		for _, w := range warnings {
			slog.Warn("Generate Caddy configuration warning.", "warn", w.String())
		}
	}

	configBytes, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal Caddy configuration: %w", err)
	}

	if err = os.WriteFile(c.path, configBytes, 0640); err != nil {
		return fmt.Errorf("write Caddy configuration to file '%s': %w", c.path, err)
	}
	if err = fs.Chown(c.path, "", CaddyGroup); err != nil {
		return fmt.Errorf("change owner of Caddy configuration file '%s': %w", c.path, err)
	}

	return nil
}
