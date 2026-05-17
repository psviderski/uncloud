package dns

import (
	"net/netip"
	"reflect"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
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
