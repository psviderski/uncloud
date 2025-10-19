package connector

import (
	"testing"

	"github.com/psviderski/uncloud/internal/machine"
	"github.com/stretchr/testify/assert"
)

func TestSSHCLIConnector_buildSSHArgs(t *testing.T) {
	tests := []struct {
		name     string
		config   SSHCLIConnectorConfig
		expected []string
	}{
		{
			name: "basic connection",
			config: SSHCLIConnectorConfig{
				User: "core",
				Host: "example.com",
			},
			expected: []string{"core@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "with custom port",
			config: SSHCLIConnectorConfig{
				User: "core",
				Host: "example.com",
				Port: 2222,
			},
			expected: []string{"-p", "2222", "core@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "with identity file",
			config: SSHCLIConnectorConfig{
				User:    "core",
				Host:    "example.com",
				KeyPath: "/path/to/key",
			},
			expected: []string{"-i", "/path/to/key", "core@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "with custom socket path",
			config: SSHCLIConnectorConfig{
				User:     "core",
				Host:     "example.com",
				SockPath: "/custom/path/uncloud.sock",
			},
			expected: []string{"core@example.com", "uncloudd", "dial-stdio", "--socket", "/custom/path/uncloud.sock"},
		},
		{
			name: "with default socket path (not included)",
			config: SSHCLIConnectorConfig{
				User:     "core",
				Host:     "example.com",
				SockPath: machine.DefaultUncloudSockPath,
			},
			expected: []string{"core@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "all options combined",
			config: SSHCLIConnectorConfig{
				User:     "core",
				Host:     "example.com",
				Port:     2222,
				KeyPath:  "/path/to/key",
				SockPath: "/custom/path/uncloud.sock",
			},
			expected: []string{"-p", "2222", "-i", "/path/to/key", "core@example.com", "uncloudd", "dial-stdio", "--socket", "/custom/path/uncloud.sock"},
		},
		{
			name: "port 22 not included (default)",
			config: SSHCLIConnectorConfig{
				User: "core",
				Host: "example.com",
				Port: 22,
			},
			expected: []string{"core@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "port 0 not included (unset)",
			config: SSHCLIConnectorConfig{
				User: "core",
				Host: "example.com",
				Port: 0,
			},
			expected: []string{"core@example.com", "uncloudd", "dial-stdio"},
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
