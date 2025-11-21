package config

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
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
			name: "unix connection",
			conn: MachineConnection{
				Unix: "/run/uncloud/uncloud.sock",
			},
			want: "unix:///run/uncloud/uncloud.sock",
		},
		{
			name: "no connection",
			conn: MachineConnection{},
			want: "unknown connection",
		},
		{
			name: "ssh connection with machine id",
			conn: MachineConnection{
				SSH:       "user@host.com",
				MachineID: "ed98b9f7575308c340263cd279e3b568",
			},
			// String() does not use MachineID; just verifying entry with it is valid
			want: "ssh://user@host.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.conn.String()
			assert.Equal(t, tt.want, got)
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
			name: "unix only - valid",
			conn: MachineConnection{
				Unix: "/path/to/socket",
			},
			wantErr: false,
		},
		{
			name:    "no connection method - error",
			conn:    MachineConnection{},
			wantErr: true,
			errMsg:  "no connection method specified",
		},
		{
			name: "ssh and ssh_cli - error",
			conn: MachineConnection{
				SSH:    "user@host",
				SSHCLI: "user@host",
			},
			wantErr: true,
			errMsg:  "only one connection method allowed",
		},
		{
			name: "ssh and unix - error",
			conn: MachineConnection{
				SSH:  "user@host",
				Unix: "/path/to/socket",
			},
			wantErr: true,
			errMsg:  "only one connection method allowed",
		},
		{
			name: "ssh and tcp - error",
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
			name: "ssh_cli and tcp - error",
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
			name: "all three - error",
			conn: MachineConnection{
				SSH:    "user@host",
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
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
