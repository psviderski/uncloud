package caddyfile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"uncloud/internal/machine/store"
)

// Controller monitors container changes in the cluster store and generates a configuration file for Caddy reverse
// proxy. The generated Caddyfile allows Caddy to route external traffic to service containers across the internal
// network.
type Controller struct {
	store *store.Store
	path  string
	mu    sync.Mutex
}

func NewController(store *store.Store, path string) (*Controller, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create parent directory for Caddyfile '%s': %w", dir, err)
	}

	return &Controller{
		store: store,
		path:  path,
	}, nil
}

func (cc *Controller) Run(ctx context.Context) error {
	return nil
}
