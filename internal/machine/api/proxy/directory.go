package proxy

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

// MachineDirectory provides access to machine information in the cluster.
type MachineDirectory interface {
	// ListMachines returns a list of all machines in the cluster.
	ListMachines(ctx context.Context) ([]*pb.MachineInfo, error)
	// ResolveMachine resolves a machine name or ID to its details.
	ResolveMachine(ctx context.Context, nameOrID string) (id, name string, ip netip.Addr, err error)
}

// Store is the interface required by MachineDirectory to access the cluster store.
type Store interface {
	ListMachines(ctx context.Context) ([]*pb.MachineInfo, error)
}

// CorrosionDirectory implements MachineDirectory using the corrosion store.
type CorrosionDirectory struct {
	store Store
}

func NewCorrosionDirectory(store Store) *CorrosionDirectory {
	return &CorrosionDirectory{store: store}
}

func (d *CorrosionDirectory) ListMachines(ctx context.Context) ([]*pb.MachineInfo, error) {
	return d.store.ListMachines(ctx)
}

func (d *CorrosionDirectory) ResolveMachine(ctx context.Context, nameOrID string) (string, string, netip.Addr, error) {
	machines, err := d.store.ListMachines(ctx)
	if err != nil {
		return "", "", netip.Addr{}, fmt.Errorf("list machines: %w", err)
	}

	for _, m := range machines {
		if m.Id == nameOrID || m.Name == nameOrID {
			ip, err := m.Network.ManagementIp.ToAddr()
			if err != nil {
				return "", "", netip.Addr{}, fmt.Errorf("invalid management IP for machine %s: %w", m.Name, err)
			}
			return m.Id, m.Name, ip, nil
		}
	}

	return "", "", netip.Addr{}, fmt.Errorf("machine not found: %s", nameOrID)
}

// IsAllMachines returns true if the machine name/ID indicates all machines in the cluster.
func IsAllMachines(nameOrID string) bool {
	return nameOrID == "*"
}
