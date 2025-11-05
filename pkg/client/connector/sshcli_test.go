package connector

import (
	"testing"

	"github.com/psviderski/uncloud/internal/machine"
	"github.com/stretchr/testify/assert"
)

func TestSSHCLIConnector_buildSSHArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   SSHConnectorConfig
		expected []string
	}{
		{
			name: "basic connection",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
			},
			expected: []string{"-o", "ConnectTimeout=5", "root@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "with custom port",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
				Port: 2222,
			},
			expected: []string{"-o", "ConnectTimeout=5", "-p", "2222", "root@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "with identity file",
			config: SSHConnectorConfig{
				User:    "root",
				Host:    "example.com",
				KeyPath: "/path/to/key",
			},
			expected: []string{"-o", "ConnectTimeout=5", "-i", "/path/to/key", "root@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "with custom socket path",
			config: SSHConnectorConfig{
				User:     "root",
				Host:     "example.com",
				SockPath: "/custom/path/uncloud.sock",
			},
			expected: []string{"-o", "ConnectTimeout=5", "root@example.com", "uncloudd", "dial-stdio", "--socket", "/custom/path/uncloud.sock"},
		},
		{
			name: "with default socket path (not included)",
			config: SSHConnectorConfig{
				User:     "root",
				Host:     "example.com",
				SockPath: machine.DefaultUncloudSockPath,
			},
			expected: []string{"-o", "ConnectTimeout=5", "root@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "all options combined",
			config: SSHConnectorConfig{
				User:     "root",
				Host:     "example.com",
				Port:     2222,
				KeyPath:  "/path/to/key",
				SockPath: "/custom/path/uncloud.sock",
			},
			expected: []string{"-o", "ConnectTimeout=5", "-p", "2222", "-i", "/path/to/key", "root@example.com", "uncloudd", "dial-stdio", "--socket", "/custom/path/uncloud.sock"},
		},
		{
			name: "port 22 not included (default)",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
				Port: 0,
			},
			expected: []string{"-o", "ConnectTimeout=5", "root@example.com", "uncloudd", "dial-stdio"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &SSHCLIConnector{config: tt.config}
			got := c.buildSSHArgs()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestSSHCLIDialer_buildDialArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   SSHConnectorConfig
		address  string
		expected []string
	}{
		{
			name: "basic connection",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
			},
			address:  "10.210.1.1:5000",
			expected: []string{"-o", "ConnectTimeout=5", "-W", "10.210.1.1:5000", "root@example.com"},
		},
		{
			name: "custom port",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
				Port: 2222,
			},
			address:  "10.210.1.1:5000",
			expected: []string{"-o", "ConnectTimeout=5", "-p", "2222", "-W", "10.210.1.1:5000", "root@example.com"},
		},
		{
			name: "with identity file",
			config: SSHConnectorConfig{
				User:    "root",
				Host:    "example.com",
				Port:    22,
				KeyPath: "/home/user/.ssh/id_rsa",
			},
			address:  "10.210.1.1:5000",
			expected: []string{"-o", "ConnectTimeout=5", "-i", "/home/user/.ssh/id_rsa", "-W", "10.210.1.1:5000", "root@example.com"},
		},
		{
			name: "custom port with identity file",
			config: SSHConnectorConfig{
				User:    "root",
				Host:    "example.com",
				Port:    2222,
				KeyPath: "/home/user/.ssh/id_rsa",
			},
			address:  "10.210.1.1:5000",
			expected: []string{"-o", "ConnectTimeout=5", "-p", "2222", "-i", "/home/user/.ssh/id_rsa", "-W", "10.210.1.1:5000", "root@example.com"},
		},
		{
			name: "zero port defaults to 22",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
				Port: 0,
			},
			address:  "10.210.1.1:5000",
			expected: []string{"-o", "ConnectTimeout=5", "-W", "10.210.1.1:5000", "root@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := &sshCLIDialer{config: tt.config}
			got := d.buildDialArgs(tt.address)
			assert.Equal(t, tt.expected, got)
		})
	}
}
