package connector

import (
	"testing"

	"github.com/psviderski/uncloud/internal/machine"
	"github.com/stretchr/testify/assert"
)

func TestSSHCLIConnector_buildSSHArgs(t *testing.T) {
	tests := []struct {
		name     string
		config   SSHConnectorConfig
		expected []string
	}{
		{
			name: "basic connection",
			config: SSHConnectorConfig{
				User: "core",
				Host: "example.com",
			},
			expected: []string{"-o", "ConnectTimeout=5", "core@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "with custom port",
			config: SSHConnectorConfig{
				User: "core",
				Host: "example.com",
				Port: 2222,
			},
			expected: []string{"-o", "ConnectTimeout=5", "-p", "2222", "core@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "with identity file",
			config: SSHConnectorConfig{
				User:    "core",
				Host:    "example.com",
				KeyPath: "/path/to/key",
			},
			expected: []string{"-o", "ConnectTimeout=5", "-i", "/path/to/key", "core@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "with custom socket path",
			config: SSHConnectorConfig{
				User:     "core",
				Host:     "example.com",
				SockPath: "/custom/path/uncloud.sock",
			},
			expected: []string{"-o", "ConnectTimeout=5", "core@example.com", "uncloudd", "dial-stdio", "--socket", "/custom/path/uncloud.sock"},
		},
		{
			name: "with default socket path (not included)",
			config: SSHConnectorConfig{
				User:     "core",
				Host:     "example.com",
				SockPath: machine.DefaultUncloudSockPath,
			},
			expected: []string{"-o", "ConnectTimeout=5", "core@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "all options combined",
			config: SSHConnectorConfig{
				User:     "core",
				Host:     "example.com",
				Port:     2222,
				KeyPath:  "/path/to/key",
				SockPath: "/custom/path/uncloud.sock",
			},
			expected: []string{"-o", "ConnectTimeout=5", "-p", "2222", "-i", "/path/to/key", "core@example.com", "uncloudd", "dial-stdio", "--socket", "/custom/path/uncloud.sock"},
		},
		{
			name: "port 22 not included (default)",
			config: SSHConnectorConfig{
				User: "core",
				Host: "example.com",
				Port: 22,
			},
			expected: []string{"-o", "ConnectTimeout=5", "core@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "port 0 not included (unset)",
			config: SSHConnectorConfig{
				User: "core",
				Host: "example.com",
				Port: 0,
			},
			expected: []string{"-o", "ConnectTimeout=5", "core@example.com", "uncloudd", "dial-stdio"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector := NewSSHCLIConnector(&tt.config)
			args := connector.buildSSHArgs()
			assert.Equal(t, tt.expected, args)
		})
	}
}
