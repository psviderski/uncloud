package connector

import (
	"strings"
	"testing"

	"github.com/psviderski/uncloud/internal/machine"
	"github.com/stretchr/testify/assert"
)

func TestSSHCLIConnector_buildSSHArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		config          SSHConnectorConfig
		controlSockPath string
		expected        []string
	}{
		{
			name: "basic connection with control socket",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
			},
			controlSockPath: "/tmp/test.sock",
			expected:        []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-T", "root@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "basic connection without control socket",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
			},
			controlSockPath: "",
			expected:        []string{"-o", "ConnectTimeout=5", "-T", "root@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "with custom port",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
				Port: 2222,
			},
			controlSockPath: "/tmp/test.sock",
			expected:        []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-T", "-p", "2222", "root@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "with identity file",
			config: SSHConnectorConfig{
				User:    "root",
				Host:    "example.com",
				KeyPath: "/path/to/key",
			},
			controlSockPath: "/tmp/test.sock",
			expected:        []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-T", "-i", "/path/to/key", "root@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "with custom socket path",
			config: SSHConnectorConfig{
				User:     "root",
				Host:     "example.com",
				SockPath: "/custom/path/uncloud.sock",
			},
			controlSockPath: "/tmp/test.sock",
			expected:        []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-T", "root@example.com", "uncloudd", "dial-stdio", "--socket", "/custom/path/uncloud.sock"},
		},
		{
			name: "with default socket path (not included)",
			config: SSHConnectorConfig{
				User:     "root",
				Host:     "example.com",
				SockPath: machine.DefaultUncloudSockPath,
			},
			controlSockPath: "/tmp/test.sock",
			expected:        []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-T", "root@example.com", "uncloudd", "dial-stdio"},
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
			controlSockPath: "/tmp/test.sock",
			expected:        []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-T", "-p", "2222", "-i", "/path/to/key", "root@example.com", "uncloudd", "dial-stdio", "--socket", "/custom/path/uncloud.sock"},
		},
		{
			name: "port 0 not included",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
				Port: 0,
			},
			controlSockPath: "/tmp/test.sock",
			expected:        []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-T", "root@example.com", "uncloudd", "dial-stdio"},
		},
		{
			name: "port 22 included when explicit",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
				Port: 22,
			},
			controlSockPath: "/tmp/test.sock",
			expected:        []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-T", "-p", "22", "root@example.com", "uncloudd", "dial-stdio"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &SSHCLIConnector{config: tt.config, controlSockPath: tt.controlSockPath}
			got := c.buildSSHArgs()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestSSHCLIDialer_buildDialArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		config          SSHConnectorConfig
		controlSockPath string
		address         string
		expected        []string
	}{
		{
			name: "basic connection without control socket",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
			},
			controlSockPath: "",
			address:         "10.210.1.1:5000",
			expected:        []string{"-o", "ConnectTimeout=5", "-T", "-W", "10.210.1.1:5000", "root@example.com"},
		},
		{
			name: "basic connection with control socket",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
			},
			controlSockPath: "/tmp/test.sock",
			address:         "10.210.1.1:5000",
			expected:        []string{"-o", "ControlMaster=no", "-o", "ControlPath=/tmp/test.sock", "-o", "ConnectTimeout=5", "-T", "-W", "10.210.1.1:5000", "root@example.com"},
		},
		{
			name: "custom port with control socket",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
				Port: 2222,
			},
			controlSockPath: "/tmp/test.sock",
			address:         "10.210.1.1:5000",
			expected:        []string{"-o", "ControlMaster=no", "-o", "ControlPath=/tmp/test.sock", "-o", "ConnectTimeout=5", "-T", "-p", "2222", "-W", "10.210.1.1:5000", "root@example.com"},
		},
		{
			name: "with identity file and control socket",
			config: SSHConnectorConfig{
				User:    "root",
				Host:    "example.com",
				Port:    22,
				KeyPath: "/home/user/.ssh/id_rsa",
			},
			controlSockPath: "/tmp/test.sock",
			address:         "10.210.1.1:5000",
			expected:        []string{"-o", "ControlMaster=no", "-o", "ControlPath=/tmp/test.sock", "-o", "ConnectTimeout=5", "-T", "-p", "22", "-i", "/home/user/.ssh/id_rsa", "-W", "10.210.1.1:5000", "root@example.com"},
		},
		{
			name: "custom port with identity file and control socket",
			config: SSHConnectorConfig{
				User:    "root",
				Host:    "example.com",
				Port:    2222,
				KeyPath: "/home/user/.ssh/id_rsa",
			},
			controlSockPath: "/tmp/test.sock",
			address:         "10.210.1.1:5000",
			expected:        []string{"-o", "ControlMaster=no", "-o", "ControlPath=/tmp/test.sock", "-o", "ConnectTimeout=5", "-T", "-p", "2222", "-i", "/home/user/.ssh/id_rsa", "-W", "10.210.1.1:5000", "root@example.com"},
		},
		{
			name: "port 0 not included",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
				Port: 0,
			},
			controlSockPath: "/tmp/test.sock",
			address:         "10.210.1.1:5000",
			expected:        []string{"-o", "ControlMaster=no", "-o", "ControlPath=/tmp/test.sock", "-o", "ConnectTimeout=5", "-T", "-W", "10.210.1.1:5000", "root@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := &sshCLIDialer{config: tt.config, controlSockPath: tt.controlSockPath}
			got := d.buildDialArgs(tt.address)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestControlSocketPath(t *testing.T) {
	// Note: Cannot use t.Parallel() because a subtest uses t.Setenv().

	path1 := controlSocketPath()
	path2 := controlSocketPath()

	assert.Equal(t, path1, path2)
	assert.True(t, strings.HasSuffix(path1, ".sock"))
	assert.Contains(t, path1, "%C")

	t.Run("uses XDG_RUNTIME_DIR when set", func(t *testing.T) {
		runDir := "/user/runtime/dir"
		t.Setenv("XDG_RUNTIME_DIR", runDir)

		path := controlSocketPath()
		assert.True(t, strings.HasPrefix(path, runDir))
	})
}
