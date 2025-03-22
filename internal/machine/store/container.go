package store

import (
	"context"
	"encoding/json"
	"fmt"
	sq "github.com/Masterminds/squirrel"
	"log/slog"
	"strings"
	"time"
	"github.com/psviderski/uncloud/internal/api"
)

const (
	// SyncStatusSynced indicates that a container record is synchronised with the Docker daemon. The record may
	// become outdated even when the status is "synced" if the machine crashes or a network partition occurs.
	// The cluster membership state of the machine should also be checked to determine if the record can be trusted.
	SyncStatusSynced = "synced"
	// SyncStatusOutdated indicates that a container record may be outdated, for example, due to being unable
	// to retrieve the container's state from the Docker daemon or when the machine is being stopped or restarted.
	SyncStatusOutdated = "outdated"
)

type ContainerRecord struct {
	Container  api.Container
	MachineID  string
	SyncStatus string
	UpdatedAt  time.Time
}

type ListOptions struct {
	// MachineIDs filters containers by the machine IDs they are running on.
	MachineIDs      []string
	ServiceIDOrName ServiceIDOrNameOptions
}

// ServiceIDOrNameOptions filters containers by the service ID or name they are part of. If both ID and Name are
// provided, they are combined with an OR operator.
type ServiceIDOrNameOptions struct {
	ID   string
	Name string
}

type DeleteOptions struct {
	IDs []string
}

// CreateOrUpdateContainer creates a new container record or updates an existing one in the store database.
// The container is associated with the given machine ID that indicates which machine the container is running on.
func (s *Store) CreateOrUpdateContainer(ctx context.Context, ctr api.Container, machineID string) error {
	// Remove the environment variables from the container record before storing it in the database
	// to avoid leaking secrets.
	ctr.Config.Env = nil
	cJSON, err := json.Marshal(ctr)
	if err != nil {
		return fmt.Errorf("marshal container: %w", err)
	}

	// Insert or update the container record if the container or machine ID has changed.
	res, err := s.corro.ExecContext(ctx, `
		INSERT INTO containers (id, container, machine_id, sync_status, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'))
		ON CONFLICT (id) DO UPDATE SET container   = excluded.container,
									   machine_id  = excluded.machine_id,
									   sync_status = excluded.sync_status,
									   updated_at  = excluded.updated_at
		WHERE containers.container != excluded.container
		  OR containers.machine_id != excluded.machine_id`,
		ctr.ID, string(cJSON), machineID, SyncStatusSynced)
	if err != nil {
		return fmt.Errorf("upsert query: %w", err)
	}
	if res.RowsAffected > 0 {
		slog.Debug("Container record updated in store DB.", "id", ctr.ID, "machine_id", machineID)
	}

	return nil
}

// ListContainers returns a list of container records from the store database that match the given options.
func (s *Store) ListContainers(ctx context.Context, opts ListOptions) ([]ContainerRecord, error) {
	q := sq.Select("container", "machine_id", "sync_status", "updated_at").From("containers").
		Where(sq.Eq{"sync_status": SyncStatusSynced})

	if len(opts.MachineIDs) > 0 {
		q = q.Where(sq.Eq{"machine_id": opts.MachineIDs})
	}

	if opts.ServiceIDOrName.ID != "" || opts.ServiceIDOrName.Name != "" {
		var conditions []sq.Sqlizer
		if opts.ServiceIDOrName.ID != "" {
			conditions = append(conditions, sq.Eq{"service_id": opts.ServiceIDOrName.ID})
		}
		if opts.ServiceIDOrName.Name != "" {
			conditions = append(conditions, sq.Eq{"service_name": opts.ServiceIDOrName.Name})
		}
		q = q.Where(sq.Or(conditions))
	}

	query, args, err := q.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.corro.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select query: %w", err)
	}
	defer rows.Close()

	var containers []ContainerRecord
	var cJSON, machineID, syncStatus, updatedAtStr string
	var updatedAt time.Time

	for rows.Next() {
		if err = rows.Scan(&cJSON, &machineID, &syncStatus, &updatedAtStr); err != nil {
			return nil, fmt.Errorf("scan container record: %w", err)
		}

		var c api.Container
		if err = json.Unmarshal([]byte(cJSON), &c); err != nil {
			return nil, fmt.Errorf("unmarshal container: %w", err)
		}
		if updatedAt, err = time.Parse(time.DateTime, updatedAtStr); err != nil {
			return nil, fmt.Errorf("parse updated_at: %w", err)
		}
		containers = append(containers, ContainerRecord{
			Container:  c,
			MachineID:  machineID,
			SyncStatus: syncStatus,
			UpdatedAt:  updatedAt,
		})
	}

	return containers, nil
}

// DeleteContainers deletes container records from the store database that match the given options.
func (s *Store) DeleteContainers(ctx context.Context, opts DeleteOptions) error {
	query := "DELETE FROM containers"
	var args []any

	if len(opts.IDs) > 0 {
		query += " WHERE id IN (?" + strings.Repeat(", ?", len(opts.IDs)-1) + ")"
		args = make([]any, len(opts.IDs))
		for i, id := range opts.IDs {
			args[i] = id
		}
	}

	res, err := s.corro.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete query: %w", err)
	}
	if res.RowsAffected > 0 {
		slog.Debug("Container records deleted from store DB.", "ids", opts.IDs, "count", res.RowsAffected)
	}

	return nil
}

// SubscribeContainers returns a list of containers and a channel that signals changes to the list. The channel doesn't
// receive any values, it just signals when a container(s) has been added, updated, or deleted in the database.
func (s *Store) SubscribeContainers(ctx context.Context) ([]ContainerRecord, <-chan struct{}, error) {
	// TODO: figure out whether we need sync_status at all.
	q := sq.Select("container", "machine_id", "sync_status", "updated_at").From("containers").
		Where(sq.Eq{"sync_status": SyncStatusSynced})
	query, args, err := q.ToSql()
	if err != nil {
		return nil, nil, fmt.Errorf("build query: %w", err)
	}

	sub, err := s.corro.SubscribeContext(ctx, query, args, false)
	if err != nil {
		return nil, nil, err
	}

	var containers []ContainerRecord
	var cJSON, updatedAtStr string

	rows := sub.Rows()
	for rows.Next() {
		var cr ContainerRecord
		if err = rows.Scan(&cJSON, &cr.MachineID, &cr.SyncStatus, &updatedAtStr); err != nil {
			return nil, nil, err
		}

		if err = json.Unmarshal([]byte(cJSON), &cr.Container); err != nil {
			return nil, nil, fmt.Errorf("unmarshal container: %w", err)
		}
		if cr.UpdatedAt, err = time.Parse(time.DateTime, updatedAtStr); err != nil {
			return nil, nil, fmt.Errorf("parse updated_at: %w", err)
		}
		containers = append(containers, cr)
	}
	events, err := sub.Changes()
	if err != nil {
		return nil, nil, fmt.Errorf("get subscription changes: %w", err)
	}

	changes := make(chan struct{})
	go func() {
		defer close(changes)
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-events:
				if !ok {
					// events channel has been closed.
					if sub.Err() != nil {
						slog.Error("Containers subscription failed.", "id", sub.ID(), "err", sub.Err())
					}
					return
				}
				// Just signal that there is a change in the containers list.
				changes <- struct{}{}
			}
		}
	}()

	return containers, changes, nil
}
