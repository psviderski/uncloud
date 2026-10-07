package cluster

import (
	"context"
	"testing"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAddMachine_InvalidName(t *testing.T) {
	t.Parallel()

	ready := make(chan struct{})
	close(ready)
	// Invalid names must be rejected before any store access.
	c := NewCluster(nil, nil, nil, ready)
	for _, name := range []string{"VPS1", "worker.example", "rr", "nearest", "c337f00600de51ef4375c9a9a267dba5"} {
		t.Run("add", func(t *testing.T) {
			resp, err := c.AddMachine(context.Background(), &pb.AddMachineRequest{Name: name})
			require.Nil(t, resp)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			require.ErrorContains(t, err, "invalid machine name")
		})
	}
}
