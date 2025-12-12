package main

import (
	"context"
	"encoding/json"
	"net/netip"
	"testing"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// mockDockerClient implements pb.DockerClient
type mockDockerClient struct {
	pb.DockerClient // Embed to avoid implementing all methods
	listResp        *pb.ListServiceContainersResponse
	listErr         error
}

func (m *mockDockerClient) ListServiceContainers(ctx context.Context, in *pb.ListServiceContainersRequest, opts ...grpc.CallOption) (*pb.ListServiceContainersResponse, error) {
	return m.listResp, m.listErr
}

// mockClusterClient implements pb.ClusterClient
type mockClusterClient struct {
	pb.ClusterClient // Embed to avoid implementing all methods
	machinesResp     *pb.ListMachinesResponse
	machinesErr      error
}

func (m *mockClusterClient) ListMachines(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*pb.ListMachinesResponse, error) {
	return m.machinesResp, m.machinesErr
}

func TestCollectContainers_NilMetadata(t *testing.T) {
	// Setup container data
	containerData := map[string]interface{}{
		"Id":   "container1",
		"Name": "test-container",
		"Config": map[string]interface{}{
			"Image": "test-image",
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
	containerJSON, err := json.Marshal(containerData)
	require.NoError(t, err)

	serviceSpecJSON, err := json.Marshal(map[string]interface{}{})
	require.NoError(t, err)

	// Setup mocks
	mockDocker := &mockDockerClient{
		listResp: &pb.ListServiceContainersResponse{
			Messages: []*pb.MachineServiceContainers{
				{
					Metadata: nil, // Simulating the issue: nil metadata
					Containers: []*pb.ServiceContainer{
						{
							Container:   containerJSON,
							ServiceSpec: serviceSpecJSON,
						},
					},
				},
			},
		},
	}

	machineIP := "10.0.0.1"
	mockCluster := &mockClusterClient{
		machinesResp: &pb.ListMachinesResponse{
			Machines: []*pb.MachineMember{
				{
					Machine: &pb.MachineInfo{
						Name: "machine-1",
						Network: &pb.NetworkConfig{
							ManagementIp: pb.NewIP(netip.MustParseAddr(machineIP)),
						},
					},
					State: pb.MachineMember_UP,
				},
			},
		},
	}

	// Construct client with mocks
	cli := &client.Client{
		Docker: &docker.Client{
			GRPCClient: mockDocker,
		},
		ClusterClient: mockCluster,
	}

	// Execute
	containers, err := collectContainers(context.Background(), cli)
	require.NoError(t, err)

	// Verify
	assert.Len(t, containers, 1)
	if len(containers) > 0 {
		c := containers[0]
		assert.Equal(t, "container1", c.id)
		assert.Equal(t, "test-container", c.name)
		assert.Equal(t, "machine-1", c.machineName, "Should fall back to the single machine name when metadata is nil")
	}
}

func TestCollectContainers_NilMetadata_MultipleMachines_Error(t *testing.T) {
	// If we have multiple machines but receive nil metadata, it should return an error as it is ambiguous

	// Setup container data
	containerData1 := map[string]interface{}{
		"Id": "container1",
	}
	containerJSON1, _ := json.Marshal(containerData1)

	containerData2 := map[string]interface{}{
		"Id": "container2",
		"Config": map[string]interface{}{
			"Image": "test-image",
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

	// Setup mocks
	mockDocker := &mockDockerClient{
		listResp: &pb.ListServiceContainersResponse{
			Messages: []*pb.MachineServiceContainers{
				{
					Metadata: nil, // Nil metadata
					Containers: []*pb.ServiceContainer{
						{
							Container:   containerJSON1,
							ServiceSpec: serviceSpecJSON,
						},
					},
				},
				{
					Metadata: &pb.Metadata{Machine: "10.0.0.2"},
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

	mockCluster := &mockClusterClient{
		machinesResp: &pb.ListMachinesResponse{
			Machines: []*pb.MachineMember{
				{
					Machine: &pb.MachineInfo{
						Name: "machine-1",
						Network: &pb.NetworkConfig{
							ManagementIp: pb.NewIP(netip.MustParseAddr("10.0.0.1")),
						},
					},
				},
				{
					Machine: &pb.MachineInfo{
						Name: "machine-2",
						Network: &pb.NetworkConfig{
							ManagementIp: pb.NewIP(netip.MustParseAddr("10.0.0.2")),
						},
					},
				},
			},
		},
	}

	// Construct client with mocks
	cli := &client.Client{
		Docker: &docker.Client{
			GRPCClient: mockDocker,
		},
		ClusterClient: mockCluster,
	}

	// Execute
	_, err := collectContainers(context.Background(), cli)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metadata is missing for a machine response")
}

func TestCollectContainers_MetadataPresent_MultipleMachines(t *testing.T) {
	// Verify correct mapping of containers to machines when metadata is present

	// Setup container data
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

	// Setup mocks
	mockDocker := &mockDockerClient{
		listResp: &pb.ListServiceContainersResponse{
			Messages: []*pb.MachineServiceContainers{
				{
					Metadata: &pb.Metadata{Machine: "10.0.0.1"},
					Containers: []*pb.ServiceContainer{
						{
							Container:   containerJSON1,
							ServiceSpec: serviceSpecJSON,
						},
					},
				},
				{
					Metadata: &pb.Metadata{Machine: "10.0.0.2"},
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

	mockCluster := &mockClusterClient{
		machinesResp: &pb.ListMachinesResponse{
			Machines: []*pb.MachineMember{
				{
					Machine: &pb.MachineInfo{
						Name: "machine-1",
						Network: &pb.NetworkConfig{
							ManagementIp: pb.NewIP(netip.MustParseAddr("10.0.0.1")),
						},
					},
				},
				{
					Machine: &pb.MachineInfo{
						Name: "machine-2",
						Network: &pb.NetworkConfig{
							ManagementIp: pb.NewIP(netip.MustParseAddr("10.0.0.2")),
						},
					},
				},
			},
		},
	}

	cli := &client.Client{
		Docker:        &docker.Client{GRPCClient: mockDocker},
		ClusterClient: mockCluster,
	}

	containers, err := collectContainers(context.Background(), cli)
	require.NoError(t, err)

	assert.Len(t, containers, 2)
	// Order is not guaranteed by the map iteration in logic or parallel fetch (though here it's mocked sequential),
	// but collectContainers just appends.
	// We'll find them by ID.
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

func TestCollectContainers_NilMetadata_NoMachines(t *testing.T) {
	// Case: 1 msc with nil metadata but no machines at all

	containerData := map[string]interface{}{
		"Id": "container1",
		"Config": map[string]interface{}{
			"Image": "test-image",
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
	containerJSON, _ := json.Marshal(containerData)
	serviceSpecJSON, _ := json.Marshal(map[string]interface{}{})

	mockDocker := &mockDockerClient{
		listResp: &pb.ListServiceContainersResponse{
			Messages: []*pb.MachineServiceContainers{
				{
					Metadata: nil,
					Containers: []*pb.ServiceContainer{
						{
							Container:   containerJSON,
							ServiceSpec: serviceSpecJSON,
						},
					},
				},
			},
		},
	}

	// No machines in cluster response
	mockCluster := &mockClusterClient{
		machinesResp: &pb.ListMachinesResponse{
			Machines: []*pb.MachineMember{},
		},
	}

	cli := &client.Client{
		Docker:        &docker.Client{GRPCClient: mockDocker},
		ClusterClient: mockCluster,
	}

	containers, err := collectContainers(context.Background(), cli)
	require.NoError(t, err)

	assert.Len(t, containers, 1)
	if len(containers) > 0 {
		assert.Equal(t, "unknown", containers[0].machineName)
	}
}

func TestCollectContainers_MetadataPresent_NotInMapping(t *testing.T) {
	// Case: msc with metadata that is not in the IP-to-name mapping

	containerData := map[string]interface{}{
		"Id": "container1",
		"Config": map[string]interface{}{
			"Image": "test-image",
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
	containerJSON, _ := json.Marshal(containerData)
	serviceSpecJSON, _ := json.Marshal(map[string]interface{}{})

	mockDocker := &mockDockerClient{
		listResp: &pb.ListServiceContainersResponse{
			Messages: []*pb.MachineServiceContainers{
				{
					Metadata: &pb.Metadata{Machine: "10.0.0.99"}, // Unknown IP
					Containers: []*pb.ServiceContainer{
						{
							Container:   containerJSON,
							ServiceSpec: serviceSpecJSON,
						},
					},
				},
			},
		},
	}

	mockCluster := &mockClusterClient{
		machinesResp: &pb.ListMachinesResponse{
			Machines: []*pb.MachineMember{
				{
					Machine: &pb.MachineInfo{
						Name: "machine-1",
						Network: &pb.NetworkConfig{
							ManagementIp: pb.NewIP(netip.MustParseAddr("10.0.0.1")),
						},
					},
				},
			},
		},
	}

	cli := &client.Client{
		Docker:        &docker.Client{GRPCClient: mockDocker},
		ClusterClient: mockCluster,
	}

	containers, err := collectContainers(context.Background(), cli)
	require.NoError(t, err)

	assert.Len(t, containers, 1)
	if len(containers) > 0 {
		// Should fallback to the IP/string in metadata
		assert.Equal(t, "10.0.0.99", containers[0].machineName)
	}
}
