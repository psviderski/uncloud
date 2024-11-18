package docker

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"log/slog"
	"time"
	"uncloud/internal/machine/store"
)

const (
	NetworkName = "uncloud"
	UserChain   = "DOCKER-USER"
	// EventsDebounceInterval defines how long to wait before processing the next Docker event. Multiple events
	// occurring within this window will be processed together as a single event to prevent system overload.
	EventsDebounceInterval = 100 * time.Millisecond
	// SyncInterval defines a regular interval to sync containers to the cluster store.
	SyncInterval = 30 * time.Second

	// SyncStatusSynced indicates that a container record is synchronised with the Docker state.
	SyncStatusSynced = "synced"
	// SyncStatusOutdated indicates that a container record may be outdated, for example, due to being unable
	// to retrieve the container's state from the Docker daemon or when the machine is being stopped or restarted.
	SyncStatusOutdated = "outdated"
)

type Manager struct {
	client *client.Client
}

func NewManager(client *client.Client) *Manager {
	return &Manager{client: client}
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

func (d *Manager) WatchAndSyncContainers(ctx context.Context, store *store.Store) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Filter only local container events.
	opts := events.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("scope", "local"),
			filters.Arg("type", string(events.ContainerEventType)),
		),
	}

	// Subscribe to Docker events before running the initial sync to avoid missing any events.
	eventCh, errCh := d.client.Events(ctx, opts)
	slog.Debug("Syncing containers to cluster store before processing Docker events.")
	if err := d.syncContainersToStore(ctx, store); err != nil {
		// The deferred cancel will stop the event subscription.
		return fmt.Errorf("sync containers to cluster store: %w", err)
	}

	var (
		// debouncer is used to debounce multiple Docker events into a single event sent to the debouncerCh
		// to prevent system overload.
		debouncer   *time.Timer
		debouncerCh = make(chan events.Message)
		// ticker is used to trigger a regular sync of containers to the cluster store as a fallback.
		ticker = time.NewTicker(SyncInterval)
	)
	defer ticker.Stop()

	for {
		select {
		case e := <-eventCh:
			switch e.Action {
			// Actions that may trigger a container state change.
			case events.ActionStart,
				events.ActionStop,
				events.ActionPause,
				events.ActionUnPause,
				events.ActionKill,
				events.ActionDie,
				events.ActionOOM,
				events.ActionHealthStatusHealthy,
				events.ActionHealthStatusUnhealthy:

				if debouncer == nil {
					debouncer = time.AfterFunc(EventsDebounceInterval, func() {
						debouncerCh <- e
					})
				}
			}
		case e := <-debouncerCh:
			debouncer = nil
			slog.Debug("Syncing containers to cluster store triggered by a Docker container event.",
				"container_id", e.Actor.ID,
				"container_name", e.Actor.Attributes["name"],
				"action", e.Action)

			if err := d.syncContainersToStore(ctx, store); err != nil {
				return fmt.Errorf("sync containers to cluster store: %w", err)
			}
		case <-ticker.C:
			slog.Debug("Syncing containers to cluster store triggered by a regular interval.",
				"interval", SyncInterval)
			if err := d.syncContainersToStore(ctx, store); err != nil {
				return fmt.Errorf("sync containers to cluster store: %w", err)
			}
		case err := <-errCh:
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("receive Docker event: %w", err)
		}
	}
}

func (d *Manager) syncContainersToStore(ctx context.Context, store *store.Store) error {
	// TODO: implement
	return nil
}
