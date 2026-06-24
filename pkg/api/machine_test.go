package api

import (
	"encoding/json"
	"net/netip"
	"testing"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMachineMembersList_Info(t *testing.T) {
	t.Parallel()

	publicKey := []byte{0x01, 0x02, 0x03, 0x04}
	members := MachineMembersList{
		{
			Machine: &pb.MachineInfo{
				Id:            "abc123",
				Name:          "vm-1",
				Hostname:      "vm-1.example.com",
				Arch:          "amd64",
				OsPrettyName:  "Ubuntu 24.04.4 LTS",
				KernelVersion: "6.8.0-31-generic",
				DockerVersion: "27.1.1",
				DaemonVersion: "0.9.0",
				PublicIp:      pb.NewIP(netip.MustParseAddr("203.0.113.5")),
				Network: &pb.NetworkConfig{
					Subnet:       pb.NewIPPrefix(netip.MustParsePrefix("10.210.0.0/24")),
					ManagementIp: pb.NewIP(netip.MustParseAddr("10.210.0.1")),
					Endpoints: []*pb.IPPort{
						pb.NewIPPort(netip.MustParseAddrPort("203.0.113.5:51820")),
					},
					PublicKey: publicKey,
				},
			},
			State: pb.MachineMember_UP,
		},
	}

	infos := members.ToNative()
	require.Len(t, infos, 1)

	assert.Equal(t, MachineMember{
		ID:            "abc123",
		Name:          "vm-1",
		State:         "Up",
		Hostname:      "vm-1.example.com",
		Arch:          "amd64",
		OSPrettyName:  "Ubuntu 24.04.4 LTS",
		KernelVersion: "6.8.0-31-generic",
		DockerVersion: "27.1.1",
		DaemonVersion: "0.9.0",
		PublicIP:      netip.MustParseAddr("203.0.113.5"),
		Network: MachineNetwork{
			Subnet:       netip.MustParsePrefix("10.210.0.0/24"),
			ManagementIP: netip.MustParseAddr("10.210.0.1"),
			Endpoints:    []netip.AddrPort{netip.MustParseAddrPort("203.0.113.5:51820")},
			PublicKey:    publicKey,
		},
	}, infos[0])

	// The JSON output must use PascalCase keys to stay consistent with `docker inspect` and the
	// rest of Uncloud's JSON output.
	data, err := json.Marshal(infos)
	require.NoError(t, err)
	out := string(data)
	for _, key := range []string{
		`"ID"`, `"Name"`, `"State"`, `"Hostname"`, `"Arch"`, `"OSPrettyName"`, `"KernelVersion"`,
		`"DockerVersion"`, `"DaemonVersion"`, `"PublicIP"`, `"Network"`,
		`"Subnet"`, `"ManagementIP"`, `"Endpoints"`, `"PublicKey"`,
	} {
		assert.Contains(t, out, key)
	}
	assert.Contains(t, out, `"PublicIP":"203.0.113.5"`)
	assert.Contains(t, out, `"Subnet":"10.210.0.0/24"`)
	assert.Contains(t, out, `"Endpoints":["203.0.113.5:51820"]`)
	assert.Contains(t, out, `"PublicKey":"AQIDBA=="`)
}
