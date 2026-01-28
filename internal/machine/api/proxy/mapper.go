package proxy

import (
	"context"
	"fmt"
	"slices"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

// MachineTarget represents a resolved machine target.
type MachineTarget struct {
	ID, Name, Addr string
}

// MachineMapper provides access to machine information in the cluster.
type MachineMapper interface {
	// MapMachines resolves a list of machine names/IDs (or "*") to a list of machine targets.
	MapMachines(ctx context.Context, namesOrIDs []string) ([]MachineTarget, error)
}

// Store is the interface required by MachineMapper to access the cluster store.
type Store interface {
	ListMachines(ctx context.Context) ([]*pb.MachineInfo, error)
}

// CorrosionMapper implements MachineMapper using the corrosion store.
type CorrosionMapper struct {
	store Store
}

func NewCorrosionMapper(store Store) *CorrosionMapper {
	return &CorrosionMapper{store: store}
}

func (m *CorrosionMapper) MapMachines(ctx context.Context, namesOrIDs []string) ([]MachineTarget, error) {
	machines, err := m.store.ListMachines(ctx)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}

	allTargets := make([]MachineTarget, 0, len(machines))
	for _, machine := range machines {
		ip, err := machine.Network.ManagementIp.ToAddr()
		if err != nil {
			return nil, fmt.Errorf("invalid management IP for machine %s: %w", machine.Name, err)
		}
		allTargets = append(allTargets, MachineTarget{
			ID:   machine.Id,
			Name: machine.Name,
			Addr: ip.String(),
		})
	}

	if slices.Contains(namesOrIDs, "*") {
		return allTargets, nil
	}

	// Filter targets based on namesOrIDs
	var targets []MachineTarget
	for _, t := range allTargets {
		if slices.Contains(namesOrIDs, t.ID) || slices.Contains(namesOrIDs, t.Name) {
			targets = append(targets, t)
		}
	}

	return targets, nil
}
