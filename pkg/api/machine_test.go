package api

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateMachineName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "single letter", input: "a"},
		{name: "single digit", input: "1"},
		{name: "two characters", input: "a1"},
		{name: "generated name", input: "machine-ab12"},
		{name: "hyphens and digits", input: "1-worker-2"},
		{name: "maximum length", input: strings.Repeat("a", 63)},
		{name: "maximum length with hyphens", input: "a" + strings.Repeat("-", 61) + "1"},
		{name: "machine namespace label", input: "m"},
		{name: "mode prefix", input: "nearest-worker"},
		{name: "short hexadecimal name", input: strings.Repeat("a", 31)},
		{name: "long hexadecimal name", input: strings.Repeat("a", 33)},
		{name: "non-hexadecimal 32 characters", input: strings.Repeat("g", 32)},
		{name: "empty", wantErr: "must be 1-63 characters"},
		{name: "too long", input: strings.Repeat("a", 64), wantErr: "must be 1-63 characters"},
		{name: "uppercase", input: "VPS1", wantErr: "lowercase letters"},
		{name: "leading hyphen", input: "-worker", wantErr: "starting and ending"},
		{name: "trailing hyphen", input: "worker-", wantErr: "starting and ending"},
		{name: "underscore", input: "worker_1", wantErr: "hyphens only"},
		{name: "dot", input: "worker.example", wantErr: "hyphens only"},
		{name: "space", input: "worker 1", wantErr: "hyphens only"},
		{name: "leading whitespace", input: " worker", wantErr: "hyphens only"},
		{name: "trailing whitespace", input: "worker\t", wantErr: "hyphens only"},
		{name: "non-ASCII", input: "wörker", wantErr: "lowercase letters"},
		{name: "round-robin mode", input: "rr", wantErr: "reserved for internal DNS query modes"},
		{name: "nearest mode", input: "nearest", wantErr: "reserved for internal DNS query modes"},
		{name: "machine ID", input: "c337f00600de51ef4375c9a9a267dba5", wantErr: "machine ID format"},
	}

	for _, tt := range tests {
		err := ValidateMachineName(tt.input)
		if tt.wantErr == "" {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, tt.wantErr)
		}
	}
}

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
