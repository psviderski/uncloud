package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"uncloud/internal/machine/docker/container"
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
	Container  *container.Container
	MachineID  string
	SyncStatus string
	UpdatedAt  time.Time
}

type ListOptions struct {
	// MachineIDs filters containers by the machine IDs they are running on.
	MachineIDs []string
}

type DeleteOptions struct {
	IDs []string
}

// CreateOrUpdateContainer creates a new container record or updates an existing one in the store database.
// The container is associated with the given machine ID that indicates which machine the container is running on.
func (s *Store) CreateOrUpdateContainer(ctx context.Context, c *container.Container, machineID string) error {
	cJSON, err := json.Marshal(c)
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
		c.ID, string(cJSON), machineID, SyncStatusSynced)
	if err != nil {
		return fmt.Errorf("upsert query: %w", err)
	}
	if res.RowsAffected > 0 {
		slog.Debug("Container record updated in store DB.", "id", c.ID, "machine_id", machineID)
	}

	return nil
}

// ListContainers returns a list of container records from the store database that match the given options.
func (s *Store) ListContainers(ctx context.Context, opts ListOptions) ([]*ContainerRecord, error) {
	query := "SELECT container, machine_id, sync_status, updated_at FROM containers"
	var args []any

	if len(opts.MachineIDs) > 0 {
		query += " WHERE machine_id IN (?" + strings.Repeat(", ?", len(opts.MachineIDs)-1) + ")"
		args = make([]any, len(opts.MachineIDs))
		for i, id := range opts.MachineIDs {
			args[i] = id
		}
	}

	rows, err := s.corro.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select query: %w", err)
	}
	defer rows.Close()

	var containers []*ContainerRecord
	var cJSON, machineID, syncStatus, updatedAtStr string
	var updatedAt time.Time

	for rows.Next() {
		if err = rows.Scan(&cJSON, &machineID, &syncStatus, &updatedAtStr); err != nil {
			return nil, fmt.Errorf("scan container record: %w", err)
		}

		var c container.Container
		if err = json.Unmarshal([]byte(cJSON), &c); err != nil {
			return nil, fmt.Errorf("unmarshal container: %w", err)
		}
		if updatedAt, err = time.Parse(time.DateTime, updatedAtStr); err != nil {
			return nil, fmt.Errorf("parse updated_at: %w", err)
		}
		containers = append(containers, &ContainerRecord{
			Container:  &c,
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