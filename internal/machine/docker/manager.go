package docker

import (
	"context"
	"fmt"
	"github.com/docker/docker/client"
	"log/slog"
	"time"
	"uncloud/internal/machine/store"
)

const (
	NetworkName = "uncloud"
	UserChain   = "DOCKER-USER"
)

type Manager struct {
	client *client.Client
	store  *store.Store
}

func NewManager(client *client.Client, store *store.Store) *Manager {
	return &Manager{
		client: client,
		store:  store,
	}
}

// WaitDaemonReady waits for the Docker daemon to start and be ready to serve requests.
func (d *Manager) WaitDaemonReady(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	ready, waitingLogged := false, false
	for !ready {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_, err := d.client.Ping(ctx)
			if err == nil {
				ready = true
				break
			}
			if !client.IsErrConnectionFailed(err) {
				return fmt.Errorf("connect to Docker daemon: %w", err)
			}
			if !waitingLogged {
				slog.Info("Waiting for Docker daemon to start and be ready.")
				waitingLogged = true
			}
		}
	}
	return nil
}
