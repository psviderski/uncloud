package connector

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSSHCLIConnector_buildSSHArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		config           SSHConnectorConfig
		controlSockPath  string
		useControlMaster bool
		expected         []string
	}{
		{
			name: "basic connection with control socket",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
			},
			controlSockPath:  "/tmp/test.sock",
			useControlMaster: true,
			expected:         []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-T", "root@example.com"},
		},
		{
			name: "basic connection without control socket",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
			},
			controlSockPath:  "",
			useControlMaster: true,
			expected:         []string{"-o", "ConnectTimeout=5", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-T", "root@example.com"},
		},
		{
			name: "with custom port",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
				Port: 2222,
			},
			controlSockPath:  "/tmp/test.sock",
			useControlMaster: true,
			expected:         []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-T", "-p", "2222", "root@example.com"},
		},
		{
			name: "with identity file",
			config: SSHConnectorConfig{
				User:    "root",
				Host:    "example.com",
				KeyPath: "/path/to/key",
			},
			controlSockPath:  "/tmp/test.sock",
			useControlMaster: true,
			expected:         []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-T", "-i", "/path/to/key", "root@example.com"},
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
			controlSockPath:  "/tmp/test.sock",
			useControlMaster: true,
			expected:         []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-T", "-p", "2222", "-i", "/path/to/key", "root@example.com"},
		},
		{
			name: "port 0 not included",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
				Port: 0,
			},
			controlSockPath:  "/tmp/test.sock",
			useControlMaster: true,
			expected:         []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-T", "root@example.com"},
		},
		{
			name: "port 22 included when explicit",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
				Port: 22,
			},
			controlSockPath:  "/tmp/test.sock",
			useControlMaster: true,
			expected:         []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/test.sock", "-o", "ControlPersist=10m", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-T", "-p", "22", "root@example.com"},
		},
		{
			name: "useControlMaster=false strips control socket options",
			config: SSHConnectorConfig{
				User: "root",
				Host: "example.com",
			},
			controlSockPath:  "/tmp/test.sock",
			useControlMaster: false,
			expected:         []string{"-o", "ConnectTimeout=5", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-T", "root@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &SSHCLIConnector{config: tt.config, controlSockPath: tt.controlSockPath}
			got := c.buildSSHArgs(tt.useControlMaster)
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

	t.Run("uses XDG_RUNTIME_DIR when set and exists", func(t *testing.T) {
		runDir := t.TempDir()
		t.Setenv("XDG_RUNTIME_DIR", runDir)

		path := controlSocketPath()
		assert.True(t, strings.HasPrefix(path, runDir))
	})

	t.Run("falls back when XDG_RUNTIME_DIR is set but missing", func(t *testing.T) {
		// WSL2 without systemd sets XDG_RUNTIME_DIR to a path that doesn't exist.
		t.Setenv("XDG_RUNTIME_DIR", "/nonexistent/uncloud-test-xdg")

		path := controlSocketPath()
		assert.NotEmpty(t, path)
		assert.False(t, strings.HasPrefix(path, "/nonexistent/"))
	})
}
