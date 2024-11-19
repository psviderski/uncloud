package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/encoding/protojson"
	"log/slog"
	"uncloud/internal/corrosion"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/docker/container"
)

var (
	//go:embed schema.sql
	Schema string

	ErrKeyNotFound = errors.New("key not found")
)

// Store is a cluster store backed by a distributed Corrosion database.
type Store struct {
	corro *corrosion.APIClient
}

func New(corro *corrosion.APIClient) *Store {
	return &Store{corro: corro}
}

func (s *Store) Get(ctx context.Context, key string, value any) error {
	rows, err := s.corro.QueryContext(ctx, "SELECT value FROM cluster WHERE key = ?", key)
	if err != nil {
		return err
	}
	if !rows.Next() {
		if rows.Err() != nil {
			return rows.Err()
		}
		return ErrKeyNotFound
	}
	if err = rows.Scan(value); err != nil {
		return err
	}
	return nil
}

func (s *Store) Put(ctx context.Context, key string, value any) error {
	_, err := s.corro.ExecContext(ctx, "INSERT OR REPLACE INTO cluster (key, value) VALUES (?, ?)", key, value)
	return err
}

func (s *Store) CreateMachine(ctx context.Context, m *pb.MachineInfo) error {
	mJSON, err := protojson.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal machine info: %w", err)
	}
	_, err = s.corro.ExecContext(ctx, "INSERT INTO machines (id, info) VALUES (?, ?)", m.Id, string(mJSON))
	if err != nil {
		return fmt.Errorf("insert query: %w", err)
	}
	return nil
}

func (s *Store) ListMachines(ctx context.Context) ([]*pb.MachineInfo, error) {
	rows, err := s.corro.QueryContext(ctx, "SELECT info FROM machines ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var machines []*pb.MachineInfo
	for rows.Next() {
		var mJSON string
		if err = rows.Scan(&mJSON); err != nil {
			return nil, err
		}
		var m pb.MachineInfo
		if err = protojson.Unmarshal([]byte(mJSON), &m); err != nil {
			return nil, fmt.Errorf("unmarshal machine info: %w", err)
		}
		if err = m.Network.Validate(); err != nil {
			slog.Error("Invalid network configuration for machine in store", "id", m.Id, "error", err)
			continue
		}
		machines = append(machines, &m)
	}
	return machines, nil
}

// SubscribeMachines returns a list of machines and a channel that signals changes to the list. The channel doesn't
// receive any values, it's just signals when a machine has been added, updated, or deleted in the database.
func (s *Store) SubscribeMachines(ctx context.Context) ([]*pb.MachineInfo, <-chan struct{}, error) {
	sub, err := s.corro.SubscribeContext(ctx, "SELECT info FROM machines ORDER BY name", nil, false)
	if err != nil {
		return nil, nil, err
	}

	rows := sub.Rows()
	var machines []*pb.MachineInfo
	for rows.Next() {
		var mJSON string
		if err = rows.Scan(&mJSON); err != nil {
			return nil, nil, err
		}
		var m pb.MachineInfo
		if err = protojson.Unmarshal([]byte(mJSON), &m); err != nil {
			return nil, nil, fmt.Errorf("unmarshal machine info: %w", err)
		}
		machines = append(machines, &m)
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
			case <-events:
				// Just signal that there is a change in the machines list.
				changes <- struct{}{}
			}
		}
	}()

	return machines, changes, nil
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
