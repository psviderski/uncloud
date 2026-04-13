package sshexec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSSHCLIRemote_newSSHCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		user     string
		host     string
		port     int
		keyPath  string
		cmd      string
		expected []string
	}{
		{
			name: "host only",
			host: "example.com",
			cmd:  "whoami",
			expected: []string{
				"ssh", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new", "-T", "example.com", "whoami",
			},
		},
		{
			name: "with user",
			user: "root",
			host: "example.com",
			cmd:  "whoami",
			expected: []string{
				"ssh", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new", "-T", "root@example.com", "whoami",
			},
		},
		{
			name: "with port",
			host: "example.com",
			port: 2222,
			cmd:  "whoami",
			expected: []string{
				"ssh", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new", "-T", "-p", "2222", "example.com", "whoami",
			},
		},
		{
			name:    "with key",
			host:    "example.com",
			keyPath: "/path/to/key",
			cmd:     "whoami",
			expected: []string{
				"ssh", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new", "-T", "-i", "/path/to/key", "example.com", "whoami",
			},
		},
		{
			name: "user and port",
			user: "ubuntu",
			host: "192.168.1.10",
			port: 2222,
			cmd:  "ls -la",
			expected: []string{
				"ssh", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new", "-T", "-p", "2222", "ubuntu@192.168.1.10", "ls -la",
			},
		},
		{
			name:    "user and key",
			user:    "root",
			host:    "example.com",
			keyPath: "/path/to/key",
			cmd:     "whoami",
			expected: []string{
				"ssh", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new", "-T", "-i", "/path/to/key", "root@example.com", "whoami",
			},
		},
		{
			name:    "port and key",
			host:    "example.com",
			port:    22,
			keyPath: "~/.ssh/id_rsa",
			cmd:     "whoami",
			expected: []string{
				"ssh", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new", "-T", "-p", "22", "-i", "~/.ssh/id_rsa", "example.com", "whoami",
			},
		},
		{
			name:    "all options",
			user:    "admin",
			host:    "server.local",
			port:    2222,
			keyPath: "~/.ssh/id_rsa",
			cmd:     "sudo bash -c 'echo hello'",
			expected: []string{
				"ssh", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new", "-T", "-p", "2222", "-i", "~/.ssh/id_rsa",
				"admin@server.local", "sudo bash -c 'echo hello'",
			},
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
			execCmd := remote.newSSHCommand(context.Background(), tt.cmd)
			assert.Equal(t, tt.expected, execCmd.Args)
			assert.NotNil(t, execCmd.Cancel, "Cancel should be set for graceful shutdown")
		})
	}
}
