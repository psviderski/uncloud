package machine

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/psviderski/uncloud/internal/corrosion"
	pb "github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testLocalMachineID = "local"

type mockRTTProvider struct {
	rtts []corrosion.MemberRTTStats
	err  error
}

func (m *mockRTTProvider) MemberRTTs() ([]corrosion.MemberRTTStats, error) {
	return m.rtts, m.err
}

type mockMachineLister struct {
	machines []*pb.MachineInfo
	err      error
}

func (m *mockMachineLister) ListMachines(ctx context.Context) ([]*pb.MachineInfo, error) {
	return m.machines, m.err
}

func newMockMachine(id, mgmtIP string) *pb.MachineInfo {
	return &pb.MachineInfo{
		Id: id,
		Network: &pb.NetworkConfig{
			ManagementIp: pb.NewIP(netip.MustParseAddr(mgmtIP)),
		},
	}
}

func TestRTTCache_ByMachineID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		machineID string
		rtts      []corrosion.MemberRTTStats
		machines  []*pb.MachineInfo
		queryID   string
		wantRTT   time.Duration
		wantOK    bool
	}{
		{
			name:      "local machine returns zero RTT",
			machineID: testLocalMachineID,
			rtts:      nil,
			machines:  []*pb.MachineInfo{newMockMachine(testLocalMachineID, "10.0.0.1")},
			queryID:   testLocalMachineID,
			wantRTT:   0,
			wantOK:    true,
		},
		{
			name:      "known remote machine returns RTT",
			machineID: testLocalMachineID,
			rtts: []corrosion.MemberRTTStats{
				{Addr: netip.MustParseAddrPort("10.0.0.2:51000"), Median: 5 * time.Millisecond, StdDev: time.Millisecond},
			},
			machines: []*pb.MachineInfo{
				newMockMachine(testLocalMachineID, "10.0.0.1"),
				newMockMachine("remote", "10.0.0.2"),
			},
			queryID: "remote",
			wantRTT: 5 * time.Millisecond,
			wantOK:  true,
		},
		{
			name:      "unknown machine returns false",
			machineID: testLocalMachineID,
			rtts:      nil,
			machines:  []*pb.MachineInfo{newMockMachine(testLocalMachineID, "10.0.0.1")},
			queryID:   "unknown",
			wantRTT:   0,
			wantOK:    false,
		},
		{
			name:      "machine without network is skipped",
			machineID: testLocalMachineID,
			rtts: []corrosion.MemberRTTStats{
				{Addr: netip.MustParseAddrPort("10.0.0.2:51000"), Median: 5 * time.Millisecond, StdDev: time.Millisecond},
			},
			machines: []*pb.MachineInfo{
				newMockMachine(testLocalMachineID, "10.0.0.1"),
				{Id: "no-network"},
			},
			queryID: "no-network",
			wantRTT: 0,
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cache := newRTTCache(tt.machineID, &mockRTTProvider{rtts: tt.rtts}, &mockMachineLister{machines: tt.machines})
			err := cache.refresh(context.Background())
			require.NoError(t, err)

			rtt, ok := cache.ByMachineID(tt.queryID)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantRTT, rtt)
		})
	}
}

func TestRTTCache_LivePeerRTTs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		machineID string
		rtts      []corrosion.MemberRTTStats
		machines  []*pb.MachineInfo
		wantRTTs  map[string]RTTStats
		wantErr   bool
	}{
		{
			name:      "excludes local machine",
			machineID: testLocalMachineID,
			rtts: []corrosion.MemberRTTStats{
				{Addr: netip.MustParseAddrPort("10.0.0.2:51000"), Median: 5 * time.Millisecond, StdDev: 1 * time.Millisecond},
				{Addr: netip.MustParseAddrPort("10.0.0.3:51000"), Median: 10 * time.Millisecond, StdDev: 2 * time.Millisecond},
			},
			machines: []*pb.MachineInfo{
				newMockMachine(testLocalMachineID, "10.0.0.1"),
				newMockMachine("peer1", "10.0.0.2"),
				newMockMachine("peer2", "10.0.0.3"),
			},
			wantRTTs: map[string]RTTStats{
				"peer1": {Median: 5 * time.Millisecond, StdDev: 1 * time.Millisecond},
				"peer2": {Median: 10 * time.Millisecond, StdDev: 2 * time.Millisecond},
			},
		},
		{
			name:      "empty cluster returns empty map",
			machineID: testLocalMachineID,
			rtts:      nil,
			machines:  []*pb.MachineInfo{newMockMachine(testLocalMachineID, "10.0.0.1")},
			wantRTTs:  map[string]RTTStats{},
		},
		{
			name:      "RTT from unknown IP is ignored",
			machineID: testLocalMachineID,
			rtts: []corrosion.MemberRTTStats{
				{Addr: netip.MustParseAddrPort("10.0.0.99:51000"), Median: 5 * time.Millisecond},
			},
			machines: []*pb.MachineInfo{newMockMachine(testLocalMachineID, "10.0.0.1")},
			wantRTTs: map[string]RTTStats{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cache := newRTTCache(tt.machineID, &mockRTTProvider{rtts: tt.rtts}, &mockMachineLister{machines: tt.machines})
			result, err := cache.LivePeerRTTs(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, tt.wantRTTs, result)
		})
	}
}

func TestRTTCache_RefreshError(t *testing.T) {
	t.Parallel()

	cache := newRTTCache(testLocalMachineID, &mockRTTProvider{err: assert.AnError}, &mockMachineLister{machines: nil})
	err := cache.refresh(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get member rtts")

	cache = newRTTCache(testLocalMachineID, &mockRTTProvider{rtts: nil}, &mockMachineLister{err: assert.AnError})
	err = cache.refresh(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list machines")
}
