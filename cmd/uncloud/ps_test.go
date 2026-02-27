package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockDockerClient struct {
	pb.DockerClient
	listResp *pb.ListServiceContainersResponse
	listErr  error
}

func (m *mockDockerClient) ListServiceContainers(ctx context.Context, in *pb.ListServiceContainersRequest, opts ...grpc.CallOption) (*pb.ListServiceContainersResponse, error) {
	return m.listResp, m.listErr
}

func TestCollectContainers(t *testing.T) {
	containerData1 := map[string]interface{}{
		"Id":   "container1",
		"Name": "container-1",
		"Config": map[string]interface{}{
			"Image": "image-1",
		},
		"State": map[string]interface{}{
			"Status":     "running",
			"StartedAt":  "2023-01-01T12:00:00Z",
			"FinishedAt": "0001-01-01T00:00:00Z",
		},
		"NetworkSettings": map[string]interface{}{
			"Networks": map[string]interface{}{},
		},
	}
	containerJSON1, _ := json.Marshal(containerData1)

	containerData2 := map[string]interface{}{
		"Id":   "container2",
		"Name": "container-2",
		"Config": map[string]interface{}{
			"Image": "image-2",
		},
		"State": map[string]interface{}{
			"Status":     "running",
			"StartedAt":  "2023-01-01T12:00:00Z",
			"FinishedAt": "0001-01-01T00:00:00Z",
		},
		"NetworkSettings": map[string]interface{}{
			"Networks": map[string]interface{}{},
		},
	}
	containerJSON2, _ := json.Marshal(containerData2)

	serviceSpecJSON, _ := json.Marshal(map[string]interface{}{})

	mockDocker := &mockDockerClient{
		listResp: &pb.ListServiceContainersResponse{
			Messages: []*pb.MachineServiceContainers{
				{
					Metadata: &pb.Metadata{MachineAddr: "10.0.0.1", MachineName: "machine-1"},
					Containers: []*pb.ServiceContainer{
						{
							Container:   containerJSON1,
							ServiceSpec: serviceSpecJSON,
						},
					},
				},
				{
					Metadata: &pb.Metadata{MachineAddr: "10.0.0.2", MachineName: "machine-2"},
					Containers: []*pb.ServiceContainer{
						{
							Container:   containerJSON2,
							ServiceSpec: serviceSpecJSON,
						},
					},
				},
			},
		},
	}

	cli := &client.Client{
		Docker: &docker.Client{GRPCClient: mockDocker},
	}

	containers, err := collectContainers(context.Background(), cli)
	require.NoError(t, err)

	assert.Len(t, containers, 2)
	for _, c := range containers {
		if c.id == "container1" {
			assert.Equal(t, "machine-1", c.machineName)
		} else if c.id == "container2" {
			assert.Equal(t, "machine-2", c.machineName)
		} else {
			t.Errorf("unexpected container id: %s", c.id)
		}
	}
}
