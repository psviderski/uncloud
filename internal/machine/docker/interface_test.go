package docker

import (
	"errors"
	"net/netip"
	"testing"

	"github.com/docker/go-connections/nat"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPublicAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr     string
		expected bool
	}{
		{"8.8.8.8", true},
		{"203.0.113.10", true},
		{"2606:4700:4700::1111", true},
		{"2001:db8::1", true},
		{"10.0.0.1", false},
		{"172.16.5.4", false},
		{"192.168.1.10", false},
		{"127.0.0.1", false},
		{"169.254.10.1", false},
		{"100.64.0.1", false},
		{"100.127.255.254", false},
		{"0.0.0.0", false},
		{"224.0.0.1", false},
		{"::1", false},
		{"fe80::1", false},
		{"fc00::1", false},
		{"ff02::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, isPublicAddr(netip.MustParseAddr(tt.addr)))
		})
	}
}

func TestFilterPublicAddrs(t *testing.T) {
	t.Parallel()

	addrs := []netip.Addr{
		netip.MustParseAddr("10.0.0.1"),
		netip.MustParseAddr("203.0.113.10"),
		netip.MustParseAddr("127.0.0.1"),
		netip.MustParseAddr("2606:4700:4700::1111"),
		netip.MustParseAddr("fe80::1"),
		netip.MustParseAddr("100.64.0.1"),
	}

	assert.Equal(t, []string{"203.0.113.10", "2606:4700:4700::1111"}, filterPublicAddrs(addrs))
	assert.Empty(t, filterPublicAddrs([]netip.Addr{netip.MustParseAddr("192.168.0.1")}))
}

func TestHostPortBindingsPublicIPs(t *testing.T) {
	orig := hostPublicAddrs
	t.Cleanup(func() { hostPublicAddrs = orig })

	port := api.PortSpec{
		HostIPPublic:  true,
		PublishedPort: 53,
		ContainerPort: 53,
		Protocol:      api.ProtocolUDP,
		Mode:          api.PortModeHost,
	}

	hostPublicAddrs = func() ([]string, error) {
		return []string{"203.0.113.10", "2606:4700:4700::1111"}, nil
	}
	got, err := hostPortBindings([]api.PortSpec{port})
	require.NoError(t, err)
	assert.Equal(t, nat.PortMap{
		"53/udp": {
			{HostIP: "203.0.113.10", HostPort: "53"},
			{HostIP: "2606:4700:4700::1111", HostPort: "53"},
		},
	}, got)

	hostPublicAddrs = func() ([]string, error) {
		return nil, errors.New("no public IP addresses found on the host")
	}
	_, err = hostPortBindings([]api.PortSpec{port})
	require.ErrorContains(t, err, "no public IP addresses found")
}
