package dns

import (
	"net/netip"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterResolver_ResolveByNamespace(t *testing.T) {
	r := NewClusterResolver(nil)

	records := []store.ContainerRecord{
		{
			Container: newServiceContainer(t, "10.0.0.2", "web", "web-id", "prod"),
			MachineID: "m1",
		},
		{
			Container: newServiceContainer(t, "10.0.0.3", "web", "web-id-2", "staging"),
			MachineID: "m2",
		},
		{
			Container: newServiceContainer(t, "10.0.0.4", "db", "db-id", ""),
			MachineID: "m1",
		},
	}

	r.updateServiceIPs(records)

	assert.Equal(t,
		[]string{"10.0.0.2"},
		addrsToStrings(r.Resolve("web", "prod")),
		"should return only prod namespace IPs",
	)
	assert.Equal(t,
		[]string{"10.0.0.3"},
		addrsToStrings(r.Resolve("web", "staging")),
		"should return only staging namespace IPs",
	)
	assert.Equal(t,
		[]string{"10.0.0.4"},
		addrsToStrings(r.Resolve("db", "default")),
		"should fall back to default namespace when not set",
	)

	// Cross-namespace lookups should not find results.
	assert.Empty(t, r.Resolve("web", "default"))
}

func TestClusterResolver_GetNamespaceByIP(t *testing.T) {
	r := NewClusterResolver(nil)

	records := []store.ContainerRecord{
		{
			Container: newServiceContainer(t, "10.1.0.2", "api", "api-id", "blue"),
			MachineID: "m1",
		},
	}
	r.updateServiceIPs(records)

	ns := r.GetNamespaceByIP(parseAddr(t, "10.1.0.2"))
	require.Equal(t, "blue", ns)

	// Unknown IP should return empty string.
	assert.Equal(t, "", r.GetNamespaceByIP(parseAddr(t, "10.1.0.99")))
}

func newServiceContainer(t *testing.T, ip, name, id, namespace string) api.ServiceContainer {
	t.Helper()

	if namespace == "" {
		namespace = api.DefaultNamespace
	}

	return api.ServiceContainer{
		Container: api.Container{
			InspectResponse: container.InspectResponse{
				ContainerJSONBase: &container.ContainerJSONBase{
					State: &container.State{
						Running: true,
					},
				},
				Config: &container.Config{
					Labels: map[string]string{
						api.LabelServiceName: name,
						api.LabelServiceID:   id,
						api.LabelNamespace:   namespace,
					},
				},
				NetworkSettings: &container.NetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						api.DockerNetworkName: {
							IPAddress: ip,
						},
					},
				},
			},
		},
	}
}

func addrsToStrings(addrs []netip.Addr) []string {
	out := make([]string, len(addrs))
	for i, a := range addrs {
		out[i] = a.String()
	}
	return out
}

func parseAddr(t *testing.T, ip string) netip.Addr {
	t.Helper()
	addr, err := netip.ParseAddr(ip)
	require.NoError(t, err)
	return addr
}
