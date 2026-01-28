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
			name:     "host only",
			host:     "example.com",
			expected: []string{"-o", "ConnectTimeout=5", "example.com"},
		},
		{
			name:     "with user",
			user:     "root",
			host:     "example.com",
			expected: []string{"-o", "ConnectTimeout=5", "root@example.com"},
		},
		{
			name:     "with port",
			host:     "example.com",
			port:     2222,
			expected: []string{"-o", "ConnectTimeout=5", "-p", "2222", "example.com"},
		},
		{
			name:     "with key",
			host:     "example.com",
			keyPath:  "/path/to/key",
			expected: []string{"-o", "ConnectTimeout=5", "-i", "/path/to/key", "example.com"},
		},
		{
			name:     "user and port",
			user:     "ubuntu",
			host:     "192.168.1.10",
			port:     2222,
			expected: []string{"-o", "ConnectTimeout=5", "-p", "2222", "ubuntu@192.168.1.10"},
		},
		{
			name:     "user and key",
			user:     "root",
			host:     "example.com",
			keyPath:  "/path/to/key",
			expected: []string{"-o", "ConnectTimeout=5", "-i", "/path/to/key", "root@example.com"},
		},
		{
			name:     "port and key",
			host:     "example.com",
			port:     22,
			keyPath:  "~/.ssh/id_rsa",
			expected: []string{"-o", "ConnectTimeout=5", "-p", "22", "-i", "~/.ssh/id_rsa", "example.com"},
		},
		{
			name:     "all options",
			user:     "admin",
			host:     "server.local",
			port:     2222,
			keyPath:  "~/.ssh/id_rsa",
			expected: []string{"-o", "ConnectTimeout=5", "-p", "2222", "-i", "~/.ssh/id_rsa", "admin@server.local"},
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
