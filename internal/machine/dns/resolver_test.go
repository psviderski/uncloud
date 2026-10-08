package dns

import (
	"net/netip"
	"reflect"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestClusterResolver_UpdateServiceIPs(t *testing.T) {
	t.Parallel()

	containers := []store.ContainerRecord{
		newRecord("svc-id-1", "web", "10.210.0.2", "mach-1"),
		newRecord("svc-id-1", "web", "10.210.0.3", "mach-2"),
		newRecord("svc-id-2", "api", "10.210.1.2", "mach-1"),
	}

	r := NewClusterResolver(nil)
	r.updateServiceIPs(containers)

	firstMapPtr := reflect.ValueOf(r.serviceIPs).Pointer()
	assert.NotZero(t, firstMapPtr, "first call should populate the map")
	assert.NotEmpty(t, r.Resolve("web"))
	assert.NotEmpty(t, r.Resolve("api"))

	// Second call with the same input must not swap the map.
	r.updateServiceIPs(containers)
	assert.Equal(t, firstMapPtr, reflect.ValueOf(r.serviceIPs).Pointer(),
		"second call with identical input must not rewrite the map")

	// Reordering the input also must not rewrite the map.
	reordered := []store.ContainerRecord{containers[2], containers[0], containers[1]}
	r.updateServiceIPs(reordered)
	assert.Equal(t, firstMapPtr, reflect.ValueOf(r.serviceIPs).Pointer(),
		"reordered input with same containers must not rewrite the map")

	// A real change must rewrite the map.
	changed := append([]store.ContainerRecord{}, containers...)
	changed = append(changed, newRecord("svc-id-3", "db", "10.210.2.2", "mach-1"))
	r.updateServiceIPs(changed)
	assert.NotEqual(t, firstMapPtr, reflect.ValueOf(r.serviceIPs).Pointer(),
		"adding a new service should rewrite the map")
	assert.NotEmpty(t, r.Resolve("db"))
}

func TestClusterResolver_MachineSpecificLookups(t *testing.T) {
	t.Parallel()

	r := NewClusterResolver(nil)
	r.updateMachineIPs([]*pb.MachineInfo{
		newMachineRecord("mach-1", "mach-1-name", "10.210.0.0/24"),
		newMachineRecord("mach-2", "mach-2-name", "10.210.1.0/24"),
		// mach-3 is unknown so its containers must not register an empty-prefixed lookup.
	})
	r.updateServiceIPs([]store.ContainerRecord{
		newRecord("svc-id-1", "web", "10.210.0.2", "mach-1"),
		newRecord("svc-id-1", "web", "10.210.0.3", "mach-2"),
		newRecord("svc-id-2", "api", "10.210.1.2", "mach-1"),
		newRecord("svc-id-3", "db", "10.210.2.2", "mach-3"),
	})

	tests := []struct {
		name  string
		query string
		want  []netip.Addr
	}{
		{"machine ID", "mach-1.m.web", []netip.Addr{netip.MustParseAddr("10.210.0.2")}},
		{"machine name", "mach-1-name.m.web", []netip.Addr{netip.MustParseAddr("10.210.0.2")}},
		{"other machine name", "mach-2-name.m.web", []netip.Addr{netip.MustParseAddr("10.210.0.3")}},
		{"machine name other service", "mach-1-name.m.api", []netip.Addr{netip.MustParseAddr("10.210.1.2")}},
		{"machine name without service container", "mach-2-name.m.api", nil},
		{"machine ID without name", "mach-3.m.db", []netip.Addr{netip.MustParseAddr("10.210.2.2")}},
		{"empty machine name", ".m.db", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, r.Resolve(tt.query))
		})
	}
}

func TestClusterResolver_MachineRename(t *testing.T) {
	t.Parallel()

	webIP := []netip.Addr{netip.MustParseAddr("10.210.0.2")}

	r := NewClusterResolver(nil)
	// Containers may be received before machines.
	r.updateServiceIPs([]store.ContainerRecord{
		newRecord("svc-id-1", "web", "10.210.0.2", "mach-1"),
	})
	assert.Nil(t, r.Resolve("old-name.m.web"))

	r.updateMachineIPs([]*pb.MachineInfo{newMachineRecord("mach-1", "old-name", "10.210.0.0/24")})
	assert.Equal(t, webIP, r.Resolve("old-name.m.web"))

	// Renaming the machine must update service lookups without any container changes.
	r.updateMachineIPs([]*pb.MachineInfo{newMachineRecord("mach-1", "new-name", "10.210.0.0/24")})
	assert.Equal(t, webIP, r.Resolve("new-name.m.web"))
	assert.Nil(t, r.Resolve("old-name.m.web"))
	assert.Equal(t, webIP, r.Resolve("mach-1.m.web"))
}

func newRecord(serviceID, serviceName, ip, machineID string) store.ContainerRecord {
	return store.ContainerRecord{
		Container: api.ServiceContainer{
			Container: api.Container{
				InspectResponse: container.InspectResponse{
					ContainerJSONBase: &container.ContainerJSONBase{
						ID:    serviceName + "-" + ip,
						State: &container.State{Running: true},
					},
					NetworkSettings: &container.NetworkSettings{
						Networks: map[string]*network.EndpointSettings{
							// Hardcoded to avoid an import cycle via internal/machine/docker.
							"uncloud": {IPAddress: ip},
						},
					},
					Config: &container.Config{
						Labels: map[string]string{
							api.LabelServiceID:   serviceID,
							api.LabelServiceName: serviceName,
						},
					},
				},
			},
		},
		MachineID: machineID,
	}
}

func TestClusterResolver_UpdateMachineIPs(t *testing.T) {
	t.Parallel()

	machines := []*pb.MachineInfo{
		newMachineRecord("x0y0z0", "mach-1", "10.210.0.0/24"),
		newMachineRecord("x1y1z1", "mach-2", "10.210.1.0/24"),
		newMachineRecord("x2y2z2", "mach-3", "10.210.2.0/24"),
	}

	r := NewClusterResolver(nil)
	r.updateMachineIPs(machines)

	assert.NotEmpty(t, r.Resolve("mach-1.m"))
	assert.NotEmpty(t, r.Resolve("mach-3.m"))
	assert.NotEmpty(t, r.Resolve("x0y0z0.m"))

	assert.Equal(t, 3, len(r.Resolve("m")))
}

func newMachineRecord(machineID, machineName, prefix string) *pb.MachineInfo {
	return &pb.MachineInfo{
		Id:   machineID,
		Name: machineName,
		Network: &pb.NetworkConfig{
			Subnet: pb.NewIPPrefix(netip.MustParsePrefix(prefix)),
		},
	}
}
