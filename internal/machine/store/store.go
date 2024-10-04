package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"google.golang.org/protobuf/encoding/protojson"
	"uncloud/internal/corrosion"
	"uncloud/internal/machine/api/pb"
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
