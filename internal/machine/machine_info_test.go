package machine

import (
	"net/netip"
	"testing"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMachineInfo(t *testing.T) {
	t.Parallel()

	subnet := netip.MustParsePrefix("10.210.1.0/24")
	manageIP := netip.MustParseAddr("fdcc:1::1")
	publicKey := []byte("public-key")

	tests := []struct {
		name          string
		state         *State
		wantPublicIP  bool
		wantEndpoints int
	}{
		{
			name: "with endpoints and public IP",
			state: &State{
				ID:   "machine-id",
				Name: "machine-1",
				Network: &network.Config{
					Subnet:       subnet,
					ManagementIP: manageIP,
					PublicKey:    publicKey,
					Endpoints: []netip.AddrPort{
						netip.MustParseAddrPort("203.0.113.5:51820"),
						netip.MustParseAddrPort("10.0.0.2:51820"),
					},
				},
				PublicIP: netip.MustParseAddr("203.0.113.5"),
			},
			wantPublicIP:  true,
			wantEndpoints: 2,
		},
		{
			name: "without endpoints or public IP",
			state: &State{
				ID:   "machine-id",
				Name: "machine-1",
				Network: &network.Config{
					Subnet:       subnet,
					ManagementIP: manageIP,
					PublicKey:    publicKey,
				},
			},
			wantPublicIP:  false,
			wantEndpoints: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := &Machine{state: tt.state}
			info := m.Info()

			assert.Equal(t, tt.state.ID, info.Id)
			assert.Equal(t, tt.state.Name, info.Name)
			assert.Equal(t, publicKey, info.Network.PublicKey)

			gotSubnet, err := info.Network.Subnet.ToPrefix()
			require.NoError(t, err)
			assert.Equal(t, subnet, gotSubnet)

			gotManageIP, err := info.Network.ManagementIp.ToAddr()
			require.NoError(t, err)
			assert.Equal(t, manageIP, gotManageIP)

			assert.Len(t, info.Network.Endpoints, tt.wantEndpoints)
			// Endpoints must round-trip back to the original AddrPorts.
			for i, ep := range info.Network.Endpoints {
				ap, err := ep.ToAddrPort()
				require.NoError(t, err)
				assert.Equal(t, tt.state.Network.Endpoints[i], ap)
			}

			if tt.wantPublicIP {
				require.NotNil(t, info.PublicIp)
				gotIP, err := info.PublicIp.ToAddr()
				require.NoError(t, err)
				assert.Equal(t, tt.state.PublicIP, gotIP)
			} else {
				assert.Nil(t, info.PublicIp)
			}
		})
	}
}

func TestEndpointsToAddrPorts(t *testing.T) {
	t.Parallel()

	ap1 := netip.MustParseAddrPort("203.0.113.5:51820")
	ap2 := netip.MustParseAddrPort("10.0.0.2:51820")

	tests := []struct {
		name      string
		endpoints []*pb.IPPort
		want      []netip.AddrPort
	}{
		{"nil", nil, nil},
		{"empty", []*pb.IPPort{}, nil},
		{
			name:      "valid",
			endpoints: []*pb.IPPort{pb.NewIPPort(ap1), pb.NewIPPort(ap2)},
			want:      []netip.AddrPort{ap1, ap2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, endpointsToAddrPorts(tt.endpoints))
		})
	}
}
