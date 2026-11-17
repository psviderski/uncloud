package docker

import (
	"net/netip"
	"testing"

	"github.com/docker/go-connections/nat"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostPortBindings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		ports []api.PortSpec
		want  nat.PortMap
	}{
		{
			name: "same container port on IPv4 and IPv6",
			ports: []api.PortSpec{
				{
					HostIP:        netip.MustParseAddr("192.0.2.1"),
					PublishedPort: 2053,
					ContainerPort: 53,
					Protocol:      api.ProtocolTCP,
					Mode:          api.PortModeHost,
				},
				{
					HostIP:        netip.MustParseAddr("2001:db8::1"),
					PublishedPort: 2053,
					ContainerPort: 53,
					Protocol:      api.ProtocolTCP,
					Mode:          api.PortModeHost,
				},
			},
			want: nat.PortMap{
				"53/tcp": {
					{HostIP: "192.0.2.1", HostPort: "2053"},
					{HostIP: "2001:db8::1", HostPort: "2053"},
				},
			},
		},
		{
			name: "same container port on different published ports",
			ports: []api.PortSpec{
				{PublishedPort: 8080, ContainerPort: 80, Protocol: api.ProtocolTCP, Mode: api.PortModeHost},
				{PublishedPort: 8081, ContainerPort: 80, Protocol: api.ProtocolTCP, Mode: api.PortModeHost},
			},
			want: nat.PortMap{
				"80/tcp": {
					{HostPort: "8080"},
					{HostPort: "8081"},
				},
			},
		},
		{
			name: "tcp and udp are separate container ports",
			ports: []api.PortSpec{
				{PublishedPort: 53, ContainerPort: 53, Protocol: api.ProtocolTCP, Mode: api.PortModeHost},
				{PublishedPort: 53, ContainerPort: 53, Protocol: api.ProtocolUDP, Mode: api.PortModeHost},
			},
			want: nat.PortMap{
				"53/tcp": {{HostPort: "53"}},
				"53/udp": {{HostPort: "53"}},
			},
		},
		{
			name: "ingress ports are skipped",
			ports: []api.PortSpec{
				{PublishedPort: 443, ContainerPort: 8080, Protocol: api.ProtocolHTTPS, Mode: api.PortModeIngress},
			},
			want: nat.PortMap{},
		},
		{
			name: "host prefix together with a host IP for the same container port",
			ports: []api.PortSpec{
				{
					HostPrefix:    netip.MustParsePrefix("127.0.0.1/32"),
					PublishedPort: 2053,
					ContainerPort: 53,
					Protocol:      api.ProtocolUDP,
					Mode:          api.PortModeHost,
				},
				{
					HostIP:        netip.MustParseAddr("2001:db8::1"),
					PublishedPort: 2053,
					ContainerPort: 53,
					Protocol:      api.ProtocolUDP,
					Mode:          api.PortModeHost,
				},
			},
			want: nat.PortMap{
				"53/udp": {
					{HostIP: "127.0.0.1", HostPort: "2053"},
					{HostIP: "2001:db8::1", HostPort: "2053"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := hostPortBindings(tt.ports)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
