package compose

import (
	"context"
	"net/netip"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertStandardPortsToPortSpecs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ports    []types.ServicePortConfig
		expected []api.PortSpec
		wantErr  string
	}{
		{
			name: "multiple ports",
			ports: []types.ServicePortConfig{
				{Target: 8080, Published: "80", Protocol: "tcp", Mode: "ingress"},
				{Target: 8443, Published: "443", Protocol: "tcp", Mode: "host"},
				{Target: 5353, Published: "53", Protocol: "udp"},
				{Target: 2222, Published: "22"},
			},
			expected: []api.PortSpec{
				{ContainerPort: 8080, PublishedPort: 80, Protocol: "tcp", Mode: "ingress"},
				{ContainerPort: 8443, PublishedPort: 443, Protocol: "tcp", Mode: "host"},
				{ContainerPort: 5353, PublishedPort: 53, Protocol: "udp", Mode: "ingress"},
				{ContainerPort: 2222, PublishedPort: 22, Protocol: "tcp", Mode: "ingress"},
			},
		},
		{
			name:     "empty ports",
			ports:    []types.ServicePortConfig{},
			expected: make([]api.PortSpec, 0),
		},
		{
			name: "single port no published",
			ports: []types.ServicePortConfig{
				{Target: 8080},
			},
			expected: []api.PortSpec{
				{ContainerPort: 8080, PublishedPort: 0, Protocol: "tcp", Mode: "ingress"},
			},
		},
		{
			name: "IPv6 host IP",
			ports: []types.ServicePortConfig{
				{Target: 8080, Published: "80", Protocol: "tcp", HostIP: "::1", Mode: "host"},
			},
			expected: []api.PortSpec{
				{ContainerPort: 8080, PublishedPort: 80, Protocol: "tcp", Mode: "host", HostIP: mustParseAddr("::1")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := convertStandardPortsToPortSpecs(tt.ports)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// mustParseAddr is a helper function for tests
func mustParseAddr(s string) netip.Addr {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		panic(err)
	}
	return addr
}

func TestConvertServicePortConfigToPortSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		port     types.ServicePortConfig
		expected api.PortSpec
		wantErr  string
	}{
		{
			name: "basic port",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "tcp",
			},
			expected: api.PortSpec{
				ContainerPort: 8080,
				PublishedPort: 80,
				Protocol:      "tcp",
				Mode:          "ingress",
			},
		},
		{
			name: "port with defaults",
			port: types.ServicePortConfig{
				Target: 8080,
			},
			expected: api.PortSpec{
				ContainerPort: 8080,
				PublishedPort: 0,
				Protocol:      "tcp",
				Mode:          "ingress",
			},
		},
		{
			name: "host mode with IP",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "tcp",
				Mode:      "host",
				HostIP:    "127.0.0.1",
			},
			expected: api.PortSpec{
				ContainerPort: 8080,
				PublishedPort: 80,
				Protocol:      "tcp",
				Mode:          "host",
				HostIP:        mustParseAddr("127.0.0.1"),
			},
		},
		{
			name: "UDP protocol",
			port: types.ServicePortConfig{
				Target:    5353,
				Published: "53",
				Protocol:  "udp",
			},
			expected: api.PortSpec{
				ContainerPort: 5353,
				PublishedPort: 53,
				Protocol:      "udp",
				Mode:          "ingress",
			},
		},
		{
			name: "IPv6 host IP",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "tcp",
				Mode:      "host",
				HostIP:    "::1",
			},
			expected: api.PortSpec{
				ContainerPort: 8080,
				PublishedPort: 80,
				Protocol:      "tcp",
				Mode:          "host",
				HostIP:        mustParseAddr("::1"),
			},
		},
		{
			name: "HTTP protocol",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "http",
			},
			expected: api.PortSpec{
				ContainerPort: 8080,
				PublishedPort: 80,
				Protocol:      "http",
				Mode:          "ingress",
			},
		},
		{
			name: "HTTPS protocol",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "443",
				Protocol:  "https",
			},
			expected: api.PortSpec{
				ContainerPort: 8080,
				PublishedPort: 443,
				Protocol:      "https",
				Mode:          "ingress",
			},
		},
		// Error cases
		{
			name: "invalid published port",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "invalid",
			},
			wantErr: "invalid published port",
		},
		{
			name: "invalid host IP",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				HostIP:    "invalid",
			},
			wantErr: "invalid host IP",
		},
		{
			name: "missing container port",
			port: types.ServicePortConfig{
				Published: "80",
			},
			wantErr: "container port must be non-zero",
		},
		{
			name: "missing container port",
			port: types.ServicePortConfig{
				Published: "8000-9000",
			},
			wantErr: "port range '8000-9000' for published port is not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := convertServicePortConfigToPortSpec(tt.port)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransformServicesPortsExtension_MutualExclusivity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name: "both ports and x-ports specified",
			content: `
services:
  web:
    image: nginx
    ports:
      - "80:8080"
    x-ports:
      - "443:8443/https"
`,
			wantErr: `service "web" cannot specify both 'ports' and 'x-ports' directives, use only one`,
		},
		{
			name: "only ports specified",
			content: `
services:
  web:
    image: nginx
    ports:
      - "80:8080"
`,
		},
		{
			name: "only x-ports specified",
			content: `
services:
  web:
    image: nginx
    x-ports:
      - "80:8080/tcp@host"
`,
		},
		{
			name: "no ports specified",
			content: `
services:
  web:
    image: nginx
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			project, err := LoadProjectFromContent(context.Background(), tt.content)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)

			// Verify service exists
			service, err := project.GetService("web")
			require.NoError(t, err)

			// Check that ports were processed correctly
			if specs, ok := service.Extensions[PortsExtensionKey].([]api.PortSpec); ok {
				// Should have specs if ports were specified
				assert.NotEmpty(t, specs)
			}
		})
	}
}

func TestTransformServicesPortsExtension_StandardPorts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		expected []api.PortSpec
	}{
		{
			name: "standard ports short syntax",
			content: `
services:
  web:
    image: nginx
    ports:
      - "80:8080"
      - "443:8443/tcp"
      - "53:5353/udp"
`,
			expected: []api.PortSpec{
				{ContainerPort: 8080, PublishedPort: 80, Protocol: "tcp", Mode: "ingress"},
				{ContainerPort: 8443, PublishedPort: 443, Protocol: "tcp", Mode: "ingress"},
				{ContainerPort: 5353, PublishedPort: 53, Protocol: "udp", Mode: "ingress"},
			},
		},
		{
			name: "standard ports long syntax",
			content: `
services:
  web:
    image: nginx
    ports:
      - target: 8080
        published: 80
        protocol: tcp
        mode: ingress
      - target: 8443
        published: 443
        protocol: tcp
        mode: host
`,
			expected: []api.PortSpec{
				{ContainerPort: 8080, PublishedPort: 80, Protocol: "tcp", Mode: "ingress"},
				{ContainerPort: 8443, PublishedPort: 443, Protocol: "tcp", Mode: "host"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			project, err := LoadProjectFromContent(context.Background(), tt.content)
			require.NoError(t, err)

			service, err := project.GetService("web")
			require.NoError(t, err)

			specs, ok := service.Extensions[PortsExtensionKey].([]api.PortSpec)
			require.True(t, ok, "Service should have port specs")

			assert.ElementsMatch(t, tt.expected, specs)
		})
	}
}

func TestTransformServicesPortsExtension_XPorts(t *testing.T) {
	t.Parallel()

	content := `
services:
  web:
    image: nginx
    x-ports:
      - "80:8080/tcp"
      - "443:8443/https"
      - "9090:9090/tcp@host"
`

	project, err := LoadProjectFromContent(context.Background(), content)
	require.NoError(t, err)

	service, err := project.GetService("web")
	require.NoError(t, err)

	specs, ok := service.Extensions[PortsExtensionKey].([]api.PortSpec)
	require.True(t, ok, "Service should have port specs")

	// Just verify that x-ports still work - don't check exact values as that's tested elsewhere
	assert.Len(t, specs, 3)
}
