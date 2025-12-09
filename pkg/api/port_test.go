package api

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortSpec_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    PortSpec
		wantErr string
	}{
		// Valid ingress mode.
		{
			name: "ingress mode tcp",
			spec: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "ingress mode with published tcp port",
			spec: PortSpec{
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "ingress mode udp",
			spec: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolUDP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "ingress mode with published udp port",
			spec: PortSpec{
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolUDP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "ingress mode without hostname http",
			spec: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolHTTP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "ingress mode without hostname https",
			spec: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolHTTPS,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "ingress mode with hostname and http",
			spec: PortSpec{
				Hostname:      "app.example.com",
				ContainerPort: 8080,
				Protocol:      ProtocolHTTP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "ingress mode with hostname and https",
			spec: PortSpec{
				Hostname:      "app.example.com",
				ContainerPort: 8080,
				Protocol:      ProtocolHTTPS,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "ingress mode with hostname and published port",
			spec: PortSpec{
				Hostname:      "app.example.com",
				PublishedPort: 6443,
				ContainerPort: 8080,
				Protocol:      ProtocolHTTPS,
				Mode:          PortModeIngress,
			},
		},

		// Valid host mode.
		{
			name: "host mode tcp",
			spec: PortSpec{
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolUDP,
				Mode:          PortModeHost,
			},
		},
		{
			name: "host mode udp",
			spec: PortSpec{
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolUDP,
				Mode:          PortModeHost,
			},
		},
		{
			name: "host mode with IPv4",
			spec: PortSpec{
				HostIP:        netip.MustParseAddr("127.0.0.1"),
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeHost,
			},
		},
		{
			name: "host mode with IPv6",
			spec: PortSpec{
				HostIP:        netip.MustParseAddr("2001:db8::1234:5678"),
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolUDP,
				Mode:          PortModeHost,
			},
		},

		// Error cases.
		{
			name: "missing container port",
			spec: PortSpec{
				Protocol: ProtocolTCP,
			},
			wantErr: "container port must be non-zero",
		},
		{
			name: "invalid protocol",
			spec: PortSpec{
				ContainerPort: 8080,
				Protocol:      "invalid",
			},
			wantErr: "invalid protocol 'invalid'",
		},
		{
			name: "invalid mode",
			spec: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          "invalid",
			},
			wantErr: "invalid mode: 'invalid'",
		},
		{
			name: "hostname with tcp protocol",
			spec: PortSpec{
				Hostname:      "app.example.com",
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "hostname with tcp protocol and published port",
			spec: PortSpec{
				Hostname:      "db.example.com",
				PublishedPort: 5432,
				ContainerPort: 5432,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "invalid hostname",
			spec: PortSpec{
				Hostname:      "app",
				ContainerPort: 8080,
				Protocol:      ProtocolHTTPS,
				Mode:          PortModeIngress,
			},
			wantErr: "invalid hostname 'app': must be a valid domain name containing at least one dot",
		},
		{
			name: "host IP in ingress mode",
			spec: PortSpec{
				HostIP:        netip.MustParseAddr("127.0.0.1"),
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
			wantErr: "host IP cannot be specified in ingress mode",
		},
		{
			name: "zero published port in host mode",
			spec: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeHost,
			},
			wantErr: "published port is required in host mode",
		},
		{
			name: "hostname in host mode",
			spec: PortSpec{
				Hostname:      "app.example.com",
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeHost,
			},
			wantErr: "hostname cannot be specified in host mode",
		},
		{
			name: "http in host mode",
			spec: PortSpec{
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolHTTP,
				Mode:          PortModeHost,
			},
			wantErr: "unsupported protocol 'http' in host mode",
		},
		{
			name: "https in host mode",
			spec: PortSpec{
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolHTTPS,
				Mode:          PortModeHost,
			},
			wantErr: "unsupported protocol 'https' in host mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.spec.Validate()
			if tt.wantErr != "" {
				require.Error(t, err, tt.wantErr)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPortSpec_String(t *testing.T) {
	tests := []struct {
		name     string
		spec     PortSpec
		expected string
	}{
		// Ingress mode.
		{
			name: "container port only",
			spec: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
			expected: "8080/tcp",
		},
		{
			name: "container port udp",
			spec: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolUDP,
				Mode:          PortModeIngress,
			},
			expected: "8080/udp",
		},
		{
			name: "published and container port",
			spec: PortSpec{
				ContainerPort: 8080,
				PublishedPort: 80,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
			expected: "80:8080/tcp",
		},
		{
			name: "hostname and container port https",
			spec: PortSpec{
				Hostname:      "app.example.com",
				ContainerPort: 8080,
				Protocol:      ProtocolHTTPS,
				Mode:          PortModeIngress,
			},
			expected: "app.example.com:8080/https",
		},
		{
			name: "hostname and container port http",
			spec: PortSpec{
				Hostname:      "app.example.com",
				ContainerPort: 8080,
				Protocol:      ProtocolHTTP,
				Mode:          PortModeIngress,
			},
			expected: "app.example.com:8080/http",
		},
		{
			name: "hostname and published and container port https",
			spec: PortSpec{
				Hostname:      "app.example.com",
				PublishedPort: 6443,
				ContainerPort: 8080,
				Protocol:      ProtocolHTTPS,
				Mode:          PortModeIngress,
			},
			expected: "app.example.com:6443:8080/https",
		},
		{
			name: "hostname and published and container port http",
			spec: PortSpec{
				Hostname:      "app.example.com",
				PublishedPort: 6443,
				ContainerPort: 8080,
				Protocol:      ProtocolHTTP,
				Mode:          PortModeIngress,
			},
			expected: "app.example.com:6443:8080/http",
		},
		{
			name: "hostname and container port tcp",
			spec: PortSpec{
				Hostname:      "db.example.com",
				ContainerPort: 5432,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
			expected: "db.example.com:5432/tcp",
		},
		{
			name: "hostname and published and container port tcp",
			spec: PortSpec{
				Hostname:      "db.example.com",
				PublishedPort: 35432,
				ContainerPort: 5432,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
			expected: "db.example.com:35432:5432/tcp",
		},

		// Host mode.
		{
			name: "host mode tcp",
			spec: PortSpec{
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeHost,
			},
			expected: "80:8080/tcp@host",
		},
		{
			name: "host mode udp",
			spec: PortSpec{
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolUDP,
				Mode:          PortModeHost,
			},
			expected: "80:8080/udp@host",
		},
		{
			name: "host mode with IPv4 tcp",
			spec: PortSpec{
				HostIP:        netip.MustParseAddr("127.0.0.1"),
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeHost,
			},
			expected: "127.0.0.1:80:8080/tcp@host",
		},
		{
			name: "host mode with IPv4 udp",
			spec: PortSpec{
				HostIP:        netip.MustParseAddr("127.0.0.1"),
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolUDP,
				Mode:          PortModeHost,
			},
			expected: "127.0.0.1:80:8080/udp@host",
		},
		{
			name: "host mode with IPv6",
			spec: PortSpec{
				HostIP:        netip.MustParseAddr("2001:db8::1234:5678"),
				PublishedPort: 80,
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeHost,
			},
			expected: "[2001:db8::1234:5678]:80:8080/tcp@host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.spec.String()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

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
			name: "container port http without hostname",
			port: "8080/http",
			expected: PortSpec{
				ContainerPort: 8080,
				Protocol:      ProtocolHTTP,
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
			name:    "container port zero",
			port:    "0",
			wantErr: "container port must be non-zero",
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
			port:    "app.example.com:invalid:8080",
			wantErr: "invalid published port",
		},
		{
			name:    "invalid hostname",
			port:    "app:8080/http",
			wantErr: "invalid hostname 'app': must be a valid domain name containing at least one dot",
		},
		{
			name: "hostname with tcp protocol",
			port: "app.example.com:8080/tcp",
			expected: PortSpec{
				Hostname:      "app.example.com",
				ContainerPort: 8080,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
		},
		{
			name: "hostname with tcp and published port",
			port: "db.example.com:5433:5432/tcp",
			expected: PortSpec{
				Hostname:      "db.example.com",
				PublishedPort: 5433,
				ContainerPort: 5432,
				Protocol:      ProtocolTCP,
				Mode:          PortModeIngress,
			},
		},

		{
			name:    "missing published port in host mode",
			port:    "8080@host",
			wantErr: "published port is required in host mode",
		},
		{
			name:    "missing published port with protocol in host mode",
			port:    "8080/udp@host",
			wantErr: "published port is required in host mode",
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
			name:    "http in host mode",
			port:    "80:8080/http@host",
			wantErr: "unsupported protocol 'http' in host mode, only 'tcp' and 'udp' are supported",
		},
		{
			name:    "https in host mode",
			port:    "80:8080/https@host",
			wantErr: "unsupported protocol 'https' in host mode, only 'tcp' and 'udp' are supported",
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
