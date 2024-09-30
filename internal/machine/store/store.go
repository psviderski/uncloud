package store

import (
	_ "embed"
	"fmt"
	"uncloud/internal/corrosion"
	"uncloud/internal/machine/api/pb"
)

//go:embed schema.sql
var Schema string

// Store is a cluster store backed by a distributed Corrosion database.
type Store struct {
	corro *corrosion.APIClient
}

func New(corro *corrosion.APIClient) *Store {
	return &Store{corro: corro}
}

func (s *Store) CreateMachine(machine *pb.MachineInfo) error {
	return fmt.Errorf("not implemented")
}

func (s *Store) ListMachines() ([]*pb.MachineInfo, error) {
	return nil, fmt.Errorf("not implemented")
}
