package proxy

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

// MachineTarget represents a resolved machine target.
type MachineTarget struct {
	ID, Name, Addr string
}

// MachinesNotFoundError indicates that one or more requested machines were not found.
type MachinesNotFoundError struct {
	NotFound []string
}

func (e *MachinesNotFoundError) Error() string {
	if len(e.NotFound) == 1 {
		return fmt.Sprintf("machine not found: %s", e.NotFound[0])
	}
	return fmt.Sprintf("machines not found: %s", strings.Join(e.NotFound, ", "))
}

// MachineMapper provides access to machine information in the cluster.
type MachineMapper interface {
	// MapMachines resolves a list of machine names/IDs (or "*") to a list of machine targets.
	// Returns MachinesNotFoundError if any requested machine is not found (except when "*" is used).
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

	// Build a map for lookup (keyed by both ID and name)
	targetByLookup := make(map[string]MachineTarget, len(allTargets)*2)
	for _, t := range allTargets {
		targetByLookup[t.ID] = t
		targetByLookup[t.Name] = t
	}

	// Resolve each requested machine
	targets := make([]MachineTarget, 0, len(namesOrIDs))
	var notFound []string
	seen := make(map[string]bool, len(namesOrIDs))

	for _, nameOrID := range namesOrIDs {
		if seen[nameOrID] {
			continue
		}
		seen[nameOrID] = true

		if t, ok := targetByLookup[nameOrID]; ok {
			targets = append(targets, t)
		} else {
			notFound = append(notFound, nameOrID)
		}
	}

	if len(notFound) > 0 {
		return nil, &MachinesNotFoundError{NotFound: notFound}
	}

	return targets, nil
}
