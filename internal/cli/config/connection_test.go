package config

import (
	"net/netip"
	"testing"
)

func TestMachineConnection_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		conn MachineConnection
		want string
	}{
		{
			name: "ssh connection",
			conn: MachineConnection{
				SSH: "user@host.com",
			},
			want: "ssh://user@host.com",
		},
		{
			name: "ssh connection with port",
			conn: MachineConnection{
				SSH: "user@host.com:2222",
			},
			want: "ssh://user@host.com:2222",
		},
		{
			name: "ssh_cli connection",
			conn: MachineConnection{
				SSHCLI: "user@host.com",
			},
			want: "ssh+cli://user@host.com",
		},
		{
			name: "ssh_cli connection with port",
			conn: MachineConnection{
				SSHCLI: "user@host.com:2222",
			},
			want: "ssh+cli://user@host.com:2222",
		},
		{
			name: "tcp connection",
			conn: MachineConnection{
				TCP: func() *netip.AddrPort {
					addr := netip.MustParseAddrPort("10.0.0.1:8080")
					return &addr
				}(),
			},
			want: "tcp://10.0.0.1:8080",
		},
		{
			name: "no connection",
			conn: MachineConnection{},
			want: "unknown connection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.conn.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMachineConnection_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		conn    MachineConnection
		wantErr bool
		errMsg  string
	}{
		{
			name: "ssh only - valid",
			conn: MachineConnection{
				SSH: "user@host",
			},
			wantErr: false,
		},
		{
			name: "ssh_cli only - valid",
			conn: MachineConnection{
				SSHCLI: "user@host",
			},
			wantErr: false,
		},
		{
			name: "tcp only - valid",
			conn: MachineConnection{
				TCP: func() *netip.AddrPort {
					addr := netip.MustParseAddrPort("10.0.0.1:8080")
					return &addr
				}(),
			},
			wantErr: false,
		},
		{
			name:    "no connection method - invalid",
			conn:    MachineConnection{},
			wantErr: true,
			errMsg:  "no connection method specified",
		},
		{
			name: "ssh and ssh_cli - invalid",
			conn: MachineConnection{
				SSH:    "user@host1",
				SSHCLI: "user@host2",
			},
			wantErr: true,
			errMsg:  "only one connection method allowed",
		},
		{
			name: "ssh and tcp - invalid",
			conn: MachineConnection{
				SSH: "user@host",
				TCP: func() *netip.AddrPort {
					addr := netip.MustParseAddrPort("10.0.0.1:8080")
					return &addr
				}(),
			},
			wantErr: true,
			errMsg:  "only one connection method allowed",
		},
		{
			name: "ssh_cli and tcp - invalid",
			conn: MachineConnection{
				SSHCLI: "user@host",
				TCP: func() *netip.AddrPort {
					addr := netip.MustParseAddrPort("10.0.0.1:8080")
					return &addr
				}(),
			},
			wantErr: true,
			errMsg:  "only one connection method allowed",
		},
		{
			name: "all three - invalid",
			conn: MachineConnection{
				SSH:    "user@host1",
				SSHCLI: "user@host2",
				TCP: func() *netip.AddrPort {
					addr := netip.MustParseAddrPort("10.0.0.1:8080")
					return &addr
				}(),
			},
			wantErr: true,
			errMsg:  "only one connection method allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.conn.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want error containing %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
