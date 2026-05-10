package client

import (
	"context"
	"testing"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceLogsWithSnapshotRequiresServices(t *testing.T) {
	cli := &Client{}

	_, _, err := cli.ServiceLogsWithSnapshot(context.Background(), &ClusterSnapshot{}, "web", api.ServiceLogsOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not include services")
}

func TestServiceLogsWithSnapshotRequiresMachines(t *testing.T) {
	cli := &Client{}
	snapshot := &ClusterSnapshot{
		Services: []api.Service{{ID: "svc-web", Name: "web"}},
	}

	_, _, err := cli.ServiceLogsWithSnapshot(context.Background(), snapshot, "web", api.ServiceLogsOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not include machines")
}

func TestServiceLogsWithSnapshotDuplicateServiceName(t *testing.T) {
	cli := &Client{}
	snapshot := &ClusterSnapshot{
		Machines: api.MachineMembersList{},
		Services: []api.Service{
			{ID: "svc-a", Name: "web"},
			{ID: "svc-b", Name: "web"},
		},
	}

	_, _, err := cli.ServiceLogsWithSnapshot(context.Background(), snapshot, "web", api.ServiceLogsOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple services found with name 'web'")
}

func TestMachineLogsWithSnapshotUnknownMachine(t *testing.T) {
	cli := &Client{}
	snapshot := &ClusterSnapshot{
		Machines: api.MachineMembersList{{
			Machine: &pb.MachineInfo{
				Id:   "machine-1",
				Name: "machine-1",
			},
		}},
	}

	_, err := cli.MachineLogsWithSnapshot(
		context.Background(), snapshot, "uncloud", api.ServiceLogsOptions{Machines: []string{"missing"}},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "machines not found: missing")
}
