package proxy

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStore struct {
	machines []*pb.MachineInfo
	err      error
}

func (s *mockStore) ListMachines(_ context.Context) ([]*pb.MachineInfo, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.machines, nil
}

func machineInfo(id, name, ip string) *pb.MachineInfo {
	return &pb.MachineInfo{
		Id:   id,
		Name: name,
		Network: &pb.NetworkConfig{
			ManagementIp: pb.NewIP(netip.MustParseAddr(ip)),
		},
	}
}

func TestCorrosionMapper_MapMachines(t *testing.T) {
	ctx := context.Background()

	machines := []*pb.MachineInfo{
		machineInfo("id-1", "machine-a", "fd00::1"),
		machineInfo("id-2", "machine-b", "fd00::2"),
	}

	tests := []struct {
		name    string
		store   *mockStore
		input   []string
		want    []MachineTarget
		wantErr bool
		errMsg  string
	}{
		{
			name:  "wildcard returns all machines",
			store: &mockStore{machines: machines},
			input: []string{"*"},
			want: []MachineTarget{
				{ID: "id-1", Name: "machine-a", Addr: "fd00::1"},
				{ID: "id-2", Name: "machine-b", Addr: "fd00::2"},
			},
		},
		{
			name:  "single name",
			store: &mockStore{machines: machines},
			input: []string{"machine-a"},
			want: []MachineTarget{
				{ID: "id-1", Name: "machine-a", Addr: "fd00::1"},
			},
		},
		{
			name:  "single id",
			store: &mockStore{machines: machines},
			input: []string{"id-2"},
			want: []MachineTarget{
				{ID: "id-2", Name: "machine-b", Addr: "fd00::2"},
			},
		},
		{
			name:  "multiple mixed",
			store: &mockStore{machines: machines},
			input: []string{"machine-a", "id-2"},
			want: []MachineTarget{
				{ID: "id-1", Name: "machine-a", Addr: "fd00::1"},
				{ID: "id-2", Name: "machine-b", Addr: "fd00::2"},
			},
		},
		{
			name:  "deduplicates repeated inputs",
			store: &mockStore{machines: machines},
			input: []string{"machine-a", "machine-a"},
			want: []MachineTarget{
				{ID: "id-1", Name: "machine-a", Addr: "fd00::1"},
			},
		},
		{
			name:  "deduplicates name and id for same machine",
			store: &mockStore{machines: machines},
			input: []string{"machine-a", "id-1"},
			want: []MachineTarget{
				{ID: "id-1", Name: "machine-a", Addr: "fd00::1"},
			},
		},
		{
			name:    "not found single",
			store:   &mockStore{machines: machines},
			input:   []string{"missing"},
			wantErr: true,
			errMsg:  "machine not found: missing",
		},
		{
			name:    "not found multiple",
			store:   &mockStore{machines: machines},
			input:   []string{"missing", "also-missing"},
			wantErr: true,
			errMsg:  "machines not found: missing, also-missing",
		},
		{
			name:    "partial not found",
			store:   &mockStore{machines: machines},
			input:   []string{"machine-a", "missing"},
			wantErr: true,
			errMsg:  "machine not found: missing",
		},
		{
			name:    "wildcard with no machines",
			store:   &mockStore{machines: []*pb.MachineInfo{}},
			input:   []string{"*"},
			wantErr: true,
			errMsg:  "no machines in cluster",
		},
		{
			name:    "store error",
			store:   &mockStore{err: errors.New("store down")},
			input:   []string{"*"},
			wantErr: true,
			errMsg:  "list machines: store down",
		},
		{
			name:    "invalid management ip",
			store:   &mockStore{machines: []*pb.MachineInfo{{Id: "bad", Name: "bad-ip", Network: &pb.NetworkConfig{ManagementIp: &pb.IP{}}}}},
			input:   []string{"*"},
			wantErr: true,
			errMsg:  "invalid management IP for machine bad-ip",
		},
		{
			name:    "empty input returns error",
			store:   &mockStore{machines: machines},
			input:   []string{},
			wantErr: true,
			errMsg:  "no machines specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewCorrosionMapper(tt.store)
			got, err := mapper.MapMachines(ctx, tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
