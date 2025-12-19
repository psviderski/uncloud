package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/psviderski/uncloud/internal/corrosion"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"google.golang.org/protobuf/encoding/protojson"
)

var (
	//go:embed schema.sql
	Schema string

	ErrKeyNotFound     = errors.New("key not found")
	ErrMachineNotFound = errors.New("machine not found")
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

func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.corro.ExecContext(ctx, "DELETE FROM cluster WHERE key = ?", key)
	return err
}

// DBVersion returns the current cr-sqlite database version (Lamport timestamp).
func (s *Store) DBVersion(ctx context.Context) (int64, error) {
	rows, err := s.corro.QueryContext(ctx, "SELECT crsql_db_version()")
	if err != nil {
		return 0, fmt.Errorf("query crsql_db_version(): %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return 0, fmt.Errorf("no result from crsql_db_version()")
	}

	var version int64
	if err = rows.Scan(&version); err != nil {
		return 0, fmt.Errorf("scan db version: %w", err)
	}
	return version, nil
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

func (s *Store) GetMachine(ctx context.Context, machineID string) (*pb.MachineInfo, error) {
	if machineID == "" {
		return nil, fmt.Errorf("machine ID cannot be empty")
	}

	rows, err := s.corro.QueryContext(ctx, "SELECT info FROM machines WHERE id = ?", machineID)
	if err != nil {
		return nil, fmt.Errorf("query machine: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("query error: %w", err)
		}
		return nil, fmt.Errorf("%w: %s", ErrMachineNotFound, machineID)
	}

	var mJSON string
	if err = rows.Scan(&mJSON); err != nil {
		return nil, fmt.Errorf("scan machine info: %w", err)
	}

	if mJSON == "" {
		return nil, fmt.Errorf("machine info is empty for id %s", machineID)
	}

	protojsonParser := protojson.UnmarshalOptions{DiscardUnknown: true}
	var m pb.MachineInfo
	if err = protojsonParser.Unmarshal([]byte(mJSON), &m); err != nil {
		return nil, fmt.Errorf("unmarshal machine info for id %s: %w", machineID, err)
	}

	// Validate the unmarshaled data. just in case
	if m.Id != machineID {
		return nil, fmt.Errorf("machine ID mismatch: expected %s, got %s", machineID, m.Id)
	}

	if m.Network != nil {
		if err = m.Network.Validate(); err != nil {
			return nil, fmt.Errorf("invalid network configuration for machine %s: %w", m.Id, err)
		}
	}

	return &m, nil
}

func (s *Store) ListMachines(ctx context.Context) ([]*pb.MachineInfo, error) {
	rows, err := s.corro.QueryContext(ctx, "SELECT id, info FROM machines ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var machines []*pb.MachineInfo
	skipped := 0

	for rows.Next() {
		var id, mJSON string
		if err = rows.Scan(&id, &mJSON); err != nil {
			return nil, err
		}

		// Skip machines with empty JSON data. This can happen during partial replication
		// when cr-sqlite has created the row but the info column hasn't been synced yet.
		if mJSON == "" || mJSON == "{}" {
			slog.Debug("Skipping machine with empty data in the store (partial replication?).", "id", id)
			skipped++
			continue
		}

		protojsonParser := protojson.UnmarshalOptions{DiscardUnknown: true}
		var m pb.MachineInfo
		if err = protojsonParser.Unmarshal([]byte(mJSON), &m); err != nil {
			return nil, fmt.Errorf("unmarshal machine info: %w", err)
		}

		if err = m.Network.Validate(); err != nil {
			slog.Error("Invalid network configuration for machine in store", "id", m.Id, "err", err)
			continue
		}
		machines = append(machines, &m)
	}

	if skipped > 0 {
		slog.Warn("Listing machines from the store skipped empty records (possibly due to partial replication).",
			"skipped", skipped, "valid", len(machines))
	}

	return machines, nil
}

func (s *Store) UpdateMachine(ctx context.Context, m *pb.MachineInfo) error {
	if m == nil {
		return fmt.Errorf("machine info cannot be nil")
	}
	if m.Id == "" {
		return fmt.Errorf("machine ID cannot be empty")
	}

	mJSON, err := protojson.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal machine info: %w", err)
	}

	result, err := s.corro.ExecContext(ctx, "UPDATE machines SET info = ? WHERE id = ?", string(mJSON), m.Id)
	if err != nil {
		return fmt.Errorf("update machine: %w", err)
	}

	// Check if machine exists
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrMachineNotFound, m.Id)
	}

	return nil
}

func (s *Store) DeleteMachine(ctx context.Context, id string) error {
	result, err := s.corro.ExecContext(ctx, "DELETE FROM machines WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete machine: %w", err)
	}
	// Check if machine was deleted.
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrMachineNotFound, id)
	}

	return nil
}

// SubscribeMachines returns a list of machines and a channel that signals changes to the list. The channel doesn't
// receive any values, it just signals when a machine has been added, updated, or deleted in the database.
func (s *Store) SubscribeMachines(ctx context.Context) ([]*pb.MachineInfo, <-chan struct{}, error) {
	sub, err := s.corro.SubscribeContext(ctx, "SELECT id, info FROM machines ORDER BY name", nil, false)
	if err != nil {
		return nil, nil, err
	}

	rows := sub.Rows()
	var machines []*pb.MachineInfo
	skipped := 0

	for rows.Next() {
		var id, mJSON string
		if err = rows.Scan(&id, &mJSON); err != nil {
			return nil, nil, err
		}

		// Skip machines with empty JSON data. This can happen during partial replication
		// when cr-sqlite has created the row but the info column hasn't been synced yet.
		if mJSON == "" || mJSON == "{}" {
			slog.Debug("Skipping machine with empty data in the store (partial replication?).", "id", id)
			skipped++
			continue
		}

		var m pb.MachineInfo
		if err = protojson.Unmarshal([]byte(mJSON), &m); err != nil {
			return nil, nil, fmt.Errorf("unmarshal machine info: %w", err)
		}
		machines = append(machines, &m)
	}

	if skipped > 0 {
		slog.Warn("Machine subscription skipped empty records in the store (possibly due to partial replication).",
			"skipped", skipped, "valid", len(machines))
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
						slog.Error("Machines subscription failed.", "id", sub.ID(), "err", sub.Err())
					}
					return
				}
				// Just signal that there is a change in the machines list.
				changes <- struct{}{}
			}
		}
	}()

	return machines, changes, nil
}
