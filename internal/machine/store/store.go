package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
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

func (s *Store) CreateMachine(machine *pb.MachineInfo) error {
	return fmt.Errorf("not implemented")
}

func (s *Store) ListMachines() ([]*pb.MachineInfo, error) {
	return nil, fmt.Errorf("not implemented")
}
