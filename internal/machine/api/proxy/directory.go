package proxy

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

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

// StaticDirectory implements MachineDirectory using a static list of machines.
// It is useful when the director is used without a cluster store, e.g. in the CLI
// (although the CLI currently doesn't use the director directly, this might be useful for testing).
type StaticDirectory struct {
	Machines []*pb.MachineInfo
}

func (d *StaticDirectory) ListMachines(_ context.Context) ([]*pb.MachineInfo, error) {
	return d.Machines, nil
}

func (d *StaticDirectory) ResolveMachine(_ context.Context, nameOrID string) (string, string, netip.Addr, error) {
	for _, m := range d.Machines {
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

// MockDirectory is a mock implementation of MachineDirectory for testing.
type MockDirectory struct {
	ListFunc    func(ctx context.Context) ([]*pb.MachineInfo, error)
	ResolveFunc func(ctx context.Context, nameOrID string) (string, string, netip.Addr, error)
}

func (m *MockDirectory) ListMachines(ctx context.Context) ([]*pb.MachineInfo, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx)
	}
	return nil, nil
}

func (m *MockDirectory) ResolveMachine(ctx context.Context, nameOrID string) (string, string, netip.Addr, error) {
	if m.ResolveFunc != nil {
		return m.ResolveFunc(ctx, nameOrID)
	}
	return "", "", netip.Addr{}, fmt.Errorf("machine not found: %s", nameOrID)
}

// IsAllMachines returns true if the machine name/ID indicates all machines in the cluster.
func IsAllMachines(nameOrID string) bool {
	return nameOrID == "*" || strings.ToLower(nameOrID) == "all"
}
