package connector

import (
	"context"
	"strings"
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

func TestSSHCLIDialer_buildDialArgs(t *testing.T) {
	tests := []struct {
		name     string
		dialer   *sshCLIDialer
		address  string
		expected []string
	}{
		{
			name: "standard port with key",
			dialer: &sshCLIDialer{
				config: SSHConnectorConfig{
					User:    "testuser",
					Host:    "testhost",
					Port:    22,
					KeyPath: "/path/to/key",
				},
			},
			address: "10.210.1.1:5000",
			expected: []string{
				"-o", "ConnectTimeout=5",
				"-i", "/path/to/key",
				"-W", "10.210.1.1:5000",
				"testuser@testhost",
			},
		},
		{
			name: "custom port without key",
			dialer: &sshCLIDialer{
				config: SSHConnectorConfig{
					User: "testuser",
					Host: "testhost",
					Port: 2222,
				},
			},
			address: "10.210.1.1:5000",
			expected: []string{
				"-o", "ConnectTimeout=5",
				"-p", "2222",
				"-W", "10.210.1.1:5000",
				"testuser@testhost",
			},
		},
		{
			name: "port 0 not included",
			dialer: &sshCLIDialer{
				config: SSHConnectorConfig{
					User: "testuser",
					Host: "testhost",
					Port: 0,
				},
			},
			address: "10.210.1.1:5000",
			expected: []string{
				"-o", "ConnectTimeout=5",
				"-W", "10.210.1.1:5000",
				"testuser@testhost",
			},
		},
		{
			name: "all options combined",
			dialer: &sshCLIDialer{
				config: SSHConnectorConfig{
					User:    "testuser",
					Host:    "testhost",
					Port:    2222,
					KeyPath: "/path/to/key",
				},
			},
			address: "10.210.1.1:5000",
			expected: []string{
				"-o", "ConnectTimeout=5",
				"-p", "2222",
				"-i", "/path/to/key",
				"-W", "10.210.1.1:5000",
				"testuser@testhost",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.dialer.buildDialArgs(tt.address)
			assert.Equal(t, tt.expected, args)
		})
	}
}

func TestSSHCLIDialer_DialContext(t *testing.T) {
	dialer := &sshCLIDialer{
		config: SSHConnectorConfig{
			User:    "testuser",
			Host:    "testhost",
			Port:    2222,
			KeyPath: "/path/to/key",
		},
	}

	// Test unsupported network type
	_, err := dialer.DialContext(context.Background(), "unix", "/var/run/test.sock")
	if err == nil {
		t.Fatal("expected error for unsupported network type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported network type") {
		t.Errorf("expected 'unsupported network type' error, got: %v", err)
	}
}

func TestSSHCLIConnector_Dialer(t *testing.T) {
	t.Run("returns error when not configured", func(t *testing.T) {
		connector := &SSHCLIConnector{}
		_, err := connector.Dialer()
		if err == nil {
			t.Fatal("expected error for unconfigured connector, got nil")
		}
		if !strings.Contains(err.Error(), "not configured") {
			t.Errorf("expected 'not configured' error, got: %v", err)
		}
	})

	t.Run("returns sshCLIDialer when configured", func(t *testing.T) {
		config := &SSHConnectorConfig{
			User: "testuser",
			Host: "testhost",
		}
		connector := NewSSHCLIConnector(config)
		dialer, err := connector.Dialer()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dialer == nil {
			t.Fatal("expected dialer, got nil")
		}
		// Verify it's the right type
		_, ok := dialer.(*sshCLIDialer)
		if !ok {
			t.Errorf("expected *sshCLIDialer, got %T", dialer)
		}
	})
}
