package client

import (
	"context"
	"testing"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceLogsForServiceUsesSelectedMachinesAsFilter(t *testing.T) {
	svc := api.Service{
		ID:   "svc-web",
		Name: "web",
		Containers: []api.MachineServiceContainer{
			{
				MachineID: "machine-2",
				Container: testServiceContainer(
					"web-1", "svc-web", "web", api.ServiceModeReplicated, false,
				),
			},
		},
	}
	machines := api.MachineMembersList{
		{
			Machine: &pb.MachineInfo{
				Id:   "machine-1",
				Name: "machine-1",
			},
		},
	}

	_, _, err := (&Client{}).ServiceLogsForService(
		context.Background(), svc, machines, "web", api.ServiceLogsOptions{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no service containers found on the specified machine")
}
