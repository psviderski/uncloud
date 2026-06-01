package dns

import (
	"log/slog"
	"net/netip"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/psviderski/uncloud/internal/machine/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMachineID = "machine-1"

type mockResolver struct {
	records map[string][]ResolvedIP
}

func (m *mockResolver) Resolve(serviceName string) []ResolvedIP {
	return m.records[serviceName]
}

func TestServer_rttForResolved(t *testing.T) {
	t.Parallel()

	localSubnet := netip.MustParsePrefix("10.210.0.0/24")

	tests := []struct {
		name           string
		resolved       ResolvedIP
		rttByMachineID func(string) (time.Duration, bool)
		want           time.Duration
	}{
		{
			name:     "local subnet returns zero",
			resolved: ResolvedIP{Addr: netip.MustParseAddr("10.210.0.5"), MachineID: "remote"},
			want:     0,
		},
		{
			name:     "known remote machine returns RTT",
			resolved: ResolvedIP{Addr: netip.MustParseAddr("10.210.1.5"), MachineID: testMachineID},
			rttByMachineID: func(id string) (time.Duration, bool) {
				if id == testMachineID {
					return 10 * time.Millisecond, true
				}
				return 0, false
			},
			want: 10 * time.Millisecond,
		},
		{
			name:     "unknown remote machine returns UnknownRTT",
			resolved: ResolvedIP{Addr: netip.MustParseAddr("10.210.1.5"), MachineID: "unknown"},
			rttByMachineID: func(id string) (time.Duration, bool) {
				return 0, false
			},
			want: constants.UnknownRTT,
		},
		{
			name:     "nil rttByMachineID returns UnknownRTT",
			resolved: ResolvedIP{Addr: netip.MustParseAddr("10.210.1.5"), MachineID: testMachineID},
			want:     constants.UnknownRTT,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := &Server{
				localSubnet:    localSubnet,
				rttByMachineID: tt.rttByMachineID,
				log:            slog.Default(),
			}

			got := s.rttForResolved(tt.resolved)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestServer_handleAQuery_NearestMode(t *testing.T) {
	t.Parallel()

	localSubnet := netip.MustParsePrefix("10.210.0.0/24")

	tests := []struct {
		name           string
		resolved       []ResolvedIP
		rttByMachineID func(string) (time.Duration, bool)
		wantOrder      []netip.Addr // expected order of IPs in response
	}{
		{
			name: "local subnet IPs come first",
			resolved: []ResolvedIP{
				{Addr: netip.MustParseAddr("10.210.1.5"), MachineID: "remote-1"},
				{Addr: netip.MustParseAddr("10.210.0.5"), MachineID: "local"},
				{Addr: netip.MustParseAddr("10.210.2.5"), MachineID: "remote-2"},
			},
			rttByMachineID: func(id string) (time.Duration, bool) {
				switch id {
				case "remote-1":
					return 5 * time.Millisecond, true
				case "remote-2":
					return 10 * time.Millisecond, true
				}
				return 0, false
			},
			wantOrder: []netip.Addr{
				netip.MustParseAddr("10.210.0.5"), // local (RTT 0)
				netip.MustParseAddr("10.210.1.5"), // remote-1 (5ms)
				netip.MustParseAddr("10.210.2.5"), // remote-2 (10ms)
			},
		},
		{
			name: "sorted by RTT ascending",
			resolved: []ResolvedIP{
				{Addr: netip.MustParseAddr("10.210.3.5"), MachineID: "slow"},
				{Addr: netip.MustParseAddr("10.210.1.5"), MachineID: "fast"},
				{Addr: netip.MustParseAddr("10.210.2.5"), MachineID: "medium"},
			},
			rttByMachineID: func(id string) (time.Duration, bool) {
				switch id {
				case "fast":
					return 1 * time.Millisecond, true
				case "medium":
					return 5 * time.Millisecond, true
				case "slow":
					return 20 * time.Millisecond, true
				}
				return 0, false
			},
			wantOrder: []netip.Addr{
				netip.MustParseAddr("10.210.1.5"), // fast (1ms)
				netip.MustParseAddr("10.210.2.5"), // medium (5ms)
				netip.MustParseAddr("10.210.3.5"), // slow (20ms)
			},
		},
		{
			name: "unknown RTT sorted last",
			resolved: []ResolvedIP{
				{Addr: netip.MustParseAddr("10.210.1.5"), MachineID: "known"},
				{Addr: netip.MustParseAddr("10.210.2.5"), MachineID: "unknown"},
			},
			rttByMachineID: func(id string) (time.Duration, bool) {
				if id == "known" {
					return 5 * time.Millisecond, true
				}
				return 0, false
			},
			wantOrder: []netip.Addr{
				netip.MustParseAddr("10.210.1.5"), // known (5ms)
				netip.MustParseAddr("10.210.2.5"), // unknown (MaxInt64)
			},
		},
		{
			name: "single IP returned as-is",
			resolved: []ResolvedIP{
				{Addr: netip.MustParseAddr("10.210.1.5"), MachineID: "only"},
			},
			wantOrder: []netip.Addr{
				netip.MustParseAddr("10.210.1.5"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := &mockResolver{
				records: map[string][]ResolvedIP{
					"web": tt.resolved,
				},
			}

			s := &Server{
				localSubnet:    localSubnet,
				resolver:       resolver,
				rttByMachineID: tt.rttByMachineID,
				log:            slog.Default(),
			}

			records := s.handleAQuery("nearest.web." + InternalDomain)
			require.Len(t, records, len(tt.wantOrder))

			for i, want := range tt.wantOrder {
				aRecord, ok := records[i].(*dns.A)
				require.True(t, ok, "expected A record at index %d", i)
				got := netip.MustParseAddr(aRecord.A.String())
				assert.Equal(t, want, got, "IP at index %d", i)
			}
		})
	}
}

func TestServer_handleAQuery_RoundRobinMode(t *testing.T) {
	t.Parallel()

	localSubnet := netip.MustParsePrefix("10.210.0.0/24")

	resolved := []ResolvedIP{
		{Addr: netip.MustParseAddr("10.210.1.5"), MachineID: "m1"},
		{Addr: netip.MustParseAddr("10.210.2.5"), MachineID: "m2"},
		{Addr: netip.MustParseAddr("10.210.3.5"), MachineID: "m3"},
	}

	resolver := &mockResolver{
		records: map[string][]ResolvedIP{
			"web": resolved,
		},
	}

	s := &Server{
		localSubnet: localSubnet,
		resolver:    resolver,
		log:         slog.Default(),
	}

	records := s.handleAQuery("rr.web." + InternalDomain)
	require.Len(t, records, 3)

	// Round-robin mode should return all IPs (order is shuffled, so just check presence)
	ips := make(map[netip.Addr]bool)
	for _, r := range records {
		aRecord := r.(*dns.A)
		ips[netip.MustParseAddr(aRecord.A.String())] = true
	}
	assert.True(t, ips[netip.MustParseAddr("10.210.1.5")])
	assert.True(t, ips[netip.MustParseAddr("10.210.2.5")])
	assert.True(t, ips[netip.MustParseAddr("10.210.3.5")])
}

func TestServer_handleAQuery_NoResults(t *testing.T) {
	t.Parallel()

	resolver := &mockResolver{
		records: map[string][]ResolvedIP{},
	}

	s := &Server{
		localSubnet: netip.MustParsePrefix("10.210.0.0/24"),
		resolver:    resolver,
		log:         slog.Default(),
	}

	records := s.handleAQuery("unknown." + InternalDomain)
	assert.Empty(t, records)
}
