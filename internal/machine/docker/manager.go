package docker

import (
	"context"
	"errors"
	"fmt"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"log/slog"
	"time"
	"uncloud/internal/api"
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
)

type Manager struct {
	client *client.Client
	// machineID is the ID of the machine where the managed Docker daemon is running.
	machineID string
	store     *store.Store
}

func NewManager(client *client.Client, machineID string, store *store.Store) *Manager {
	return &Manager{
		client:    client,
		machineID: machineID,
		store:     store,
	}
}

// WaitDaemonReady waits for the Docker daemon to start and be ready to serve requests.
func (m *Manager) WaitDaemonReady(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	ready, waitingLogged := false, false
	for !ready {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_, err := m.client.Ping(ctx)
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

func (m *Manager) WatchAndSyncContainers(ctx context.Context) error {
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
	eventCh, errCh := m.client.Events(ctx, opts)
	slog.Debug("Syncing containers to cluster store before processing Docker events.")
	if err := m.syncContainersToStore(ctx); err != nil {
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
			// Actions that may trigger a container state change or creation/deletion of a container.
			case events.ActionCreate,
				events.ActionStart,
				events.ActionStop,
				events.ActionPause,
				events.ActionUnPause,
				events.ActionKill,
				events.ActionDie,
				events.ActionOOM,
				events.ActionDestroy,
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

			if err := m.syncContainersToStore(ctx); err != nil {
				return fmt.Errorf("sync containers to cluster store: %w", err)
			}
		case <-ticker.C:
			slog.Debug("Syncing containers to cluster store triggered by a regular interval.",
				"interval", SyncInterval)
			if err := m.syncContainersToStore(ctx); err != nil {
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

func (m *Manager) syncContainersToStore(ctx context.Context) error {
	storeContainers, err := m.store.ListContainers(ctx, store.ListOptions{MachineIDs: []string{m.machineID}})
	if err != nil {
		return fmt.Errorf("list containers from store: %w", err)
	}
	// List only Uncloud service containers identified by their labels.
	containers, err := m.client.ContainerList(ctx, dockercontainer.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", api.LabelServiceID),
			filters.Arg("label", api.LabelServiceName),
		),
	})
	if err != nil {
		// TODO: mark all containers as outdated in the store.
		return fmt.Errorf("list Docker containers: %w", err)
	}

	// Delete containers that are not present in the Docker daemon from the store.
	var deleteIDs []string
	for _, sc := range storeContainers {
		found := false
		for i, _ := range containers {
			if containers[i].ID == sc.Container.ID {
				found = true
				break
			}
		}
		if !found {
			deleteIDs = append(deleteIDs, sc.Container.ID)
		}
	}

	var storeErr error
	if len(deleteIDs) > 0 {
		if err = m.store.DeleteContainers(ctx, store.DeleteOptions{IDs: deleteIDs}); err != nil {
			storeErr = fmt.Errorf("delete containers from store: %w", err)
		}
	}

	// Create or update the current Docker containers in the store.
	for _, dc := range containers {
		c := &api.Container{Container: dc}
		if err = m.store.CreateOrUpdateContainer(ctx, c, m.machineID); err != nil {
			storeErr = errors.Join(storeErr, fmt.Errorf("create or update container %q: %w", c.ID, err))
		}
	}
	return storeErr
}
