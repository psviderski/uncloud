package sshexec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSSHCLIRemote_buildSSHArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		user     string
		host     string
		port     int
		keyPath  string
		expected []string
	}{
		{
			name:     "default port, no key",
			user:     "root",
			host:     "example.com",
			port:     22,
			keyPath:  "",
			expected: []string{"-o", "ConnectTimeout=5", "root@example.com"},
		},
		{
			name:     "custom port",
			user:     "ubuntu",
			host:     "192.168.1.10",
			port:     2222,
			keyPath:  "",
			expected: []string{"-o", "ConnectTimeout=5", "-p", "2222", "ubuntu@192.168.1.10"},
		},
		{
			name:     "with key path",
			user:     "root",
			host:     "example.com",
			port:     22,
			keyPath:  "/path/to/key",
			expected: []string{"-o", "ConnectTimeout=5", "-i", "/path/to/key", "root@example.com"},
		},
		{
			name:     "all options",
			user:     "admin",
			host:     "server.local",
			port:     2222,
			keyPath:  "~/.ssh/id_rsa",
			expected: []string{"-o", "ConnectTimeout=5", "-p", "2222", "-i", "~/.ssh/id_rsa", "admin@server.local"},
		},
		{
			name:     "port zero (default)",
			user:     "root",
			host:     "example.com",
			port:     0,
			keyPath:  "",
			expected: []string{"-o", "ConnectTimeout=5", "root@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			remote := &SSHCLIRemote{
				user:    tt.user,
				host:    tt.host,
				port:    tt.port,
				keyPath: tt.keyPath,
			}
			result := remote.buildSSHArgs()
			assert.Equal(t, tt.expected, result)
		})
	}
}
