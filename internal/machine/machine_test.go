package machine

import (
	"context"
	"testing"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestInitCluster_InvalidMachineName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"VPS1", "rr", "nearest", "c337f00600de51ef4375c9a9a267dba5"} {
		// Invalid names must be rejected before initializing cluster state.
		m := &Machine{state: &State{}}

		resp, err := m.InitCluster(context.Background(), &pb.InitClusterRequest{MachineName: name})
		require.Nil(t, resp)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.ErrorContains(t, err, "invalid machine name")
	}
}

func TestUpdateMachine_InvalidName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "VPS1", "worker.example", "rr", "nearest", "c337f00600de51ef4375c9a9a267dba5"} {
		m := &Machine{state: &State{ID: "machine-id", Name: "worker"}}
		resp, err := m.UpdateMachine(context.Background(), &pb.UpdateMachineRequest{Name: &name})
		require.Nil(t, resp)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.ErrorContains(t, err, "invalid machine name")
		require.Equal(t, "worker", m.state.Name)
	}
}
