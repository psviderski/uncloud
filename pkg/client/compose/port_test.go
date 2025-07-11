package compose

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertServicePortToXPortString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		port     types.ServicePortConfig
		expected string
		wantErr  string
	}{
		// Basic cases
		{
			name: "basic tcp port with published",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "tcp",
			},
			expected: "80:8080",
		},
		{
			name: "container port only",
			port: types.ServicePortConfig{
				Target: 8080,
			},
			expected: "8080",
		},
		{
			name: "udp port",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "udp",
			},
			expected: "80:8080/udp",
		},
		{
			name: "host mode tcp",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "tcp",
				Mode:      "host",
			},
			expected: "80:8080/tcp@host",
		},
		{
			name: "host mode udp",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "udp",
				Mode:      "host",
			},
			expected: "80:8080/udp@host",
		},
		{
			name: "ingress mode explicit",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "tcp",
				Mode:      "ingress",
			},
			expected: "80:8080",
		},
		{
			name: "with host IP",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "tcp",
				HostIP:    "127.0.0.1",
			},
			expected: "127.0.0.1:80:8080",
		},
		{
			name: "with host IP and host mode",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "tcp",
				HostIP:    "127.0.0.1",
				Mode:      "host",
			},
			expected: "127.0.0.1:80:8080/tcp@host",
		},

		// IPv6 support tests
		{
			name: "IPv6 address with brackets",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "tcp",
				HostIP:    "::1",
			},
			expected: "[::1]:80:8080",
		},
		{
			name: "IPv6 full address",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "tcp",
				HostIP:    "2001:db8::1",
				Mode:      "host",
			},
			expected: "[2001:db8::1]:80:8080/tcp@host",
		},

		// Protocol support tests
		{
			name: "http protocol",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "http",
			},
			expected: "80:8080/http",
		},
		{
			name: "https protocol",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "443",
				Protocol:  "https",
			},
			expected: "443:8080/https",
		},

		// Port range in single port converter should fail (ranges are expanded at higher level)
		{
			name: "port range should fail in single converter",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "3000-3005",
				Protocol:  "tcp",
			},
			wantErr: "port ranges should be expanded before string building",
		},

		// Edge cases and boundary tests
		{
			name: "port 1 (minimum)",
			port: types.ServicePortConfig{
				Target:    1,
				Published: "1",
				Protocol:  "tcp",
			},
			expected: "1:1",
		},
		{
			name: "port 65535 (maximum)",
			port: types.ServicePortConfig{
				Target:    65535,
				Published: "65535",
				Protocol:  "tcp",
			},
			expected: "65535:65535",
		},

		// Name and AppProtocol fields (should not affect output but should be preserved)
		{
			name: "with name and app_protocol",
			port: types.ServicePortConfig{
				Target:      8080,
				Published:   "80",
				Protocol:    "tcp",
				Name:        "web",
				AppProtocol: "http",
			},
			expected: "80:8080",
		},

		// Error cases
		{
			name: "missing container port",
			port: types.ServicePortConfig{
				Published: "80",
			},
			wantErr: "container port (target) is required",
		},
		{
			name: "invalid published port",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "invalid",
			},
			wantErr: "invalid published port",
		},
		{
			name: "unsupported mode",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Mode:      "invalid",
			},
			wantErr: "unsupported port mode",
		},
		{
			name: "invalid protocol",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "invalid",
			},
			wantErr: "unsupported protocol",
		},
		{
			name: "invalid IPv4 address",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				HostIP:    "999.999.999.999",
			},
			wantErr: "invalid host IP address",
		},
		{
			name: "invalid IPv6 address",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				HostIP:    "::invalid",
			},
			wantErr: "invalid host IP address",
		},
		{
			name: "port 0 (invalid)",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "0",
			},
			wantErr: "published port 0 out of valid range",
		},
		{
			name: "port too high",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "99999",
			},
			wantErr: "invalid published port",
		},
		{
			name: "host mode without published port",
			port: types.ServicePortConfig{
				Target: 8080,
				Mode:   "host",
			},
			wantErr: "published port is required in host mode",
		},
		{
			name: "invalid port range format",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "3000-3005-4000",
			},
			wantErr: "port ranges should be expanded before string building",
		},
		{
			name: "invalid port range - start >= end",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "3005-3000",
			},
			wantErr: "port ranges should be expanded before string building",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := convertServicePortToXPortString(tt.port)
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

func TestExpandPortRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		port     types.ServicePortConfig
		expected []types.ServicePortConfig
		wantErr  string
	}{
		{
			name: "single port (no range)",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "80",
				Protocol:  "tcp",
			},
			expected: []types.ServicePortConfig{
				{
					Target:    8080,
					Published: "80",
					Protocol:  "tcp",
				},
			},
		},
		{
			name: "simple port range",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "3000-3002",
				Protocol:  "tcp",
				Mode:      "host",
			},
			expected: []types.ServicePortConfig{
				{
					Target:    8080,
					Published: "3000",
					Protocol:  "tcp",
					Mode:      "host",
				},
				{
					Target:    8081,
					Published: "3001",
					Protocol:  "tcp",
					Mode:      "host",
				},
				{
					Target:    8082,
					Published: "3002",
					Protocol:  "tcp",
					Mode:      "host",
				},
			},
		},
		{
			name: "port range with host IP",
			port: types.ServicePortConfig{
				Target:    9000,
				Published: "5000-5001",
				Protocol:  "udp",
				HostIP:    "127.0.0.1",
				Mode:      "host",
			},
			expected: []types.ServicePortConfig{
				{
					Target:    9000,
					Published: "5000",
					Protocol:  "udp",
					HostIP:    "127.0.0.1",
					Mode:      "host",
				},
				{
					Target:    9001,
					Published: "5001",
					Protocol:  "udp",
					HostIP:    "127.0.0.1",
					Mode:      "host",
				},
			},
		},
		{
			name: "large port range",
			port: types.ServicePortConfig{
				Target:    8000,
				Published: "3000-3009",
				Protocol:  "tcp",
			},
			expected: []types.ServicePortConfig{
				{Target: 8000, Published: "3000", Protocol: "tcp"},
				{Target: 8001, Published: "3001", Protocol: "tcp"},
				{Target: 8002, Published: "3002", Protocol: "tcp"},
				{Target: 8003, Published: "3003", Protocol: "tcp"},
				{Target: 8004, Published: "3004", Protocol: "tcp"},
				{Target: 8005, Published: "3005", Protocol: "tcp"},
				{Target: 8006, Published: "3006", Protocol: "tcp"},
				{Target: 8007, Published: "3007", Protocol: "tcp"},
				{Target: 8008, Published: "3008", Protocol: "tcp"},
				{Target: 8009, Published: "3009", Protocol: "tcp"},
			},
		},

		// Error cases
		{
			name: "invalid range format",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "3000-3001-3002",
			},
			wantErr: "invalid port range format",
		},
		{
			name: "invalid start port",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "abc-3001",
			},
			wantErr: "invalid start port in range",
		},
		{
			name: "invalid end port",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "3000-xyz",
			},
			wantErr: "invalid end port in range",
		},
		{
			name: "start port >= end port",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "3001-3000",
			},
			wantErr: "invalid port range",
		},
		{
			name: "start port = end port",
			port: types.ServicePortConfig{
				Target:    8080,
				Published: "3000-3000",
			},
			wantErr: "invalid port range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := expandPortRange(tt.port)
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

func TestConvertStandardPortsToXPorts_WithRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ports    []types.ServicePortConfig
		expected PortsSource
		wantErr  string
	}{
		{
			name: "port range expansion",
			ports: []types.ServicePortConfig{
				{
					Target:    8080,
					Published: "3000-3002",
					Protocol:  "tcp",
				},
			},
			expected: PortsSource{"3000:8080", "3001:8081", "3002:8082"},
		},
		{
			name: "mixed single ports and ranges",
			ports: []types.ServicePortConfig{
				{
					Target:    8080,
					Published: "80",
					Protocol:  "tcp",
				},
				{
					Target:    9000,
					Published: "4000-4001",
					Protocol:  "udp",
				},
			},
			expected: PortsSource{"80:8080", "4000:9000/udp", "4001:9001/udp"},
		},
		{
			name: "port range in host mode",
			ports: []types.ServicePortConfig{
				{
					Target:    8080,
					Published: "3000-3001",
					Protocol:  "tcp",
					Mode:      "host",
					HostIP:    "127.0.0.1",
				},
			},
			expected: PortsSource{"127.0.0.1:3000:8080/tcp@host", "127.0.0.1:3001:8081/tcp@host"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := convertStandardPortsToXPorts(tt.ports)
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

func TestConvertStandardPortsToXPorts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ports    []types.ServicePortConfig
		expected PortsSource
		wantErr  string
	}{
		{
			name: "multiple ports",
			ports: []types.ServicePortConfig{
				{Target: 8080, Published: "80", Protocol: "tcp"},
				{Target: 8443, Published: "443", Protocol: "tcp", Mode: "host"},
				{Target: 5353, Published: "53", Protocol: "udp"},
			},
			expected: PortsSource{"80:8080", "443:8443/tcp@host", "53:5353/udp"},
		},
		{
			name:     "empty ports",
			ports:    []types.ServicePortConfig{},
			expected: PortsSource(nil),
		},
		{
			name: "single port no published",
			ports: []types.ServicePortConfig{
				{Target: 8080},
			},
			expected: PortsSource{"8080"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := convertStandardPortsToXPorts(tt.ports)
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

			project, err := loadProjectFromContent(t, tt.content)
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

			project, err := loadProjectFromContent(t, tt.content)
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

	project, err := loadProjectFromContent(t, content)
	require.NoError(t, err)

	service, err := project.GetService("web")
	require.NoError(t, err)

	specs, ok := service.Extensions[PortsExtensionKey].([]api.PortSpec)
	require.True(t, ok, "Service should have port specs")
	
	// Just verify that x-ports still work - don't check exact values as that's tested elsewhere
	assert.Len(t, specs, 3)
}

