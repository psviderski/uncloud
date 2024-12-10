package api

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/netip"
	"testing"
)

func TestParsePortSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		port     string
		expected PortSpec
		wantErr  string
	}{
		// Ingress mode (default).
		{
			name: "container port only",
			port: "8080",
			expected: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "container port zero",
			port: "0",
			expected: PortSpec{
				ContainerPort: 0,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "published and container port",
			port: "8000:8080",
			expected: PortSpec{
				PublishedPort: 8000,
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "published port zero",
			port: "0:8080",
			expected: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "tcp protocol explicit",
			port: "8080/tcp",
			expected: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "udp protocol",
			port: "8080/udp",
			expected: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolUDP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "published port udp",
			port: "8000:8080/udp",
			expected: PortSpec{
				PublishedPort: 8000,
				ContainerPort: 8080,
				Protocol:      ProtocolUDP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "http protocol",
			port: "8080/http",
			expected: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolHTTP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "https protocol",
			port: "8080/https",
			expected: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolHTTPS,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "published port https",
			port: "8000:8080/https",
			expected: PortSpec{
				PublishedPort: 8000,
				ContainerPort: 8080,
				Protocol:      ProtocolHTTPS,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "hostname and container port",
			port: "app.example.com:8080",
			expected: PortSpec{
				Hostname:      "app.example.com",
				ContainerPort: 8080,
				Protocol:      ProtocolHTTPS,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "hostname and container port http",
			port: "app.example.com:8080/http",
			expected: PortSpec{
				Hostname:      "app.example.com",
				ContainerPort: 8080,
				Protocol:      ProtocolHTTP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "hostname and published port",
			port: "app.example.com:6443:8080",
			expected: PortSpec{
				Hostname:      "app.example.com",
				PublishedPort: 6443,
				ContainerPort: 8080,
				Protocol:      ProtocolHTTPS,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "hostname and published port http",
			port: "app.example.com:8000:8080/http",
			expected: PortSpec{
				Hostname:      "app.example.com",
				PublishedPort: 8000,
				ContainerPort: 8080,
				Protocol:      ProtocolHTTP,
				Mode:          PortModeIngress,
			},
		},

		// Host mode.
		{
			name: "host mode",
			port: "8080@host",
			expected: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeHost,
			},
		},
		{
			name: "host mode with protocol",
			port: "8080/udp@host",
			expected: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolUDP,
				Mode:          PortModeHost,
			},
		},
		{
			name: "host mode published with protocol",
			port: "80:8080/udp@host",
			expected: PortSpec{
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolUDP,
				Mode:          PortModeHost,
			},
		},
		{
			name: "host mode with IPv4",
			port: "127.0.0.1:80:8080@host",
			expected: PortSpec{
				HostIP:        netip.MustParseAddr("127.0.0.1"),
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeHost,
			},
		},
		{
			name: "host mode with IPv6",
			port: "[2001:db8::1234:5678]:80:8080@host",
			expected: PortSpec{
				HostIP:        netip.MustParseAddr("2001:db8::1234:5678"),
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeHost,
			},
		},
		{
			name: "host mode with IP and protocol",
			port: "127.0.0.1:80:8080/udp@host",
			expected: PortSpec{
				HostIP:        netip.MustParseAddr("127.0.0.1"),
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolUDP,
				Mode:          PortModeHost,
			},
		},

		// Error cases.
		{
			name:    "empty",
			port:    "",
			wantErr: "invalid container port",
		},
		{
			name:    "invalid container port",
			port:    "invalid",
			wantErr: "invalid container port",
		},
		{
			name:    "out of range container port",
			port:    "100500",
			wantErr: "invalid container port",
		},
		{
			name:    "just protocol",
			port:    "/tcp",
			wantErr: "invalid container port",
		},
		{
			name:    "just mode",
			port:    "@host",
			wantErr: "invalid container port",
		},
		{
			name:    "multiple @ symbols",
			port:    "8080@host@host",
			wantErr: "too many '@' symbols",
		},
		{
			name:    "invalid mode",
			port:    "8080@invalid",
			wantErr: "invalid mode: 'invalid'",
		},
		{
			name:    "multiple protocols",
			port:    "8080/tcp/udp",
			wantErr: "too many '/' symbols",
		},
		{
			name:    "invalid protocol",
			port:    "8080/invalid",
			wantErr: "unsupported protocol: 'invalid'",
		},
		{
			name:    "invalid published port",
			port:    "test:invalid:8080",
			wantErr: "invalid published port",
		},
		{
			name:    "invalid host IPv4",
			port:    "300.0.0.1:80:8080@host",
			wantErr: "invalid host IP",
		},
		{
			name:    "invalid host IPv6",
			port:    "[:::1]:80:8080@host",
			wantErr: "invalid host IP",
		},
		{
			name:    "missing closing bracket in IPv6",
			port:    "[::1:80:8080@host",
			wantErr: "invalid host IP",
		},
		{
			name:    "missing brackets in IPv6",
			port:    "2001:db8::1234:5678:80:8080@host",
			wantErr: "invalid host IP",
		},
		{
			name:    "hostname in host mode",
			port:    "app.example.com:8080@host",
			wantErr: "hostname cannot be specified in host mode",
		},
		{
			name:    "hostname with invalid published port",
			port:    "app.example.com:invalid:8080@host",
			wantErr: "invalid published port",
		},
		{
			name:    "hostname with tcp protocol",
			port:    "app.example.com:8080/tcp",
			wantErr: "hostname is only valid with 'http' or 'https' protocols",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec, err := ParsePortSpec(tt.port)
			if tt.wantErr != "" {
				require.Error(t, err, "Expected error: %s, got nil, spec: %+v", tt.wantErr, spec)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, spec)
		})
	}
}
