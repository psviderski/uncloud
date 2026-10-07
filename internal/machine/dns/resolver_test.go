package dns

import (
	"net/netip"
	"reflect"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestClusterResolver_UpdateServiceIPs(t *testing.T) {
	t.Parallel()

	containers := []store.ContainerRecord{
		newRecord("svc-id-1", "web", "10.210.0.2", "mach-1", "mach-1-name"),
		newRecord("svc-id-1", "web", "10.210.0.3", "mach-2", "mach-2-name"),
		newRecord("svc-id-2", "api", "10.210.1.2", "mach-1", "mach-1-name"),
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
	changed = append(changed, newRecord("svc-id-3", "db", "10.210.2.2", "mach-1", "mach-1-name"))
	r.updateServiceIPs(changed)
	assert.NotEqual(t, firstMapPtr, reflect.ValueOf(r.serviceIPs).Pointer(),
		"adding a new service should rewrite the map")
	assert.NotEmpty(t, r.Resolve("db"))
}

func TestClusterResolver_MachineSpecificLookups(t *testing.T) {
	t.Parallel()

	r := NewClusterResolver(nil)
	r.updateServiceIPs([]store.ContainerRecord{
		newRecord("svc-id-1", "web", "10.210.0.2", "mach-1", "mach-1-name"),
		newRecord("svc-id-1", "web", "10.210.0.3", "mach-2", "mach-2-name"),
		newRecord("svc-id-2", "api", "10.210.1.2", "mach-1", "mach-1-name"),
		// A record without a machine name must not register an empty-prefixed lookup.
		newRecord("svc-id-3", "db", "10.210.2.2", "mach-3", ""),
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

func newRecord(serviceID, serviceName, ip, machineID, machineName string) store.ContainerRecord {
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
		MachineID:   machineID,
		MachineName: machineName,
	}
}
