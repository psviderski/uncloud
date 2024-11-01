package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"google.golang.org/grpc"
	"uncloud/internal/machine/api/pb"
)

// Client is a gRPC client for the Docker service that provides a similar interface to the Docker HTTP client.
type Client struct {
	conn       *grpc.ClientConn
	grpcClient pb.DockerClient
}

// NewClient creates a new Docker gRPC client with the provided gRPC connection.
func NewClient(conn *grpc.ClientConn) *Client {
	return &Client{
		conn:       conn,
		grpcClient: pb.NewDockerClient(conn),
	}
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// CreateContainer creates a new container based on the given configuration.
func (c *Client) CreateContainer(
	ctx context.Context,
	config *container.Config,
	hostConfig *container.HostConfig,
	networkingConfig *network.NetworkingConfig,
	platform *ocispec.Platform,
	name string,
) (container.CreateResponse, error) {
	var resp container.CreateResponse
	// Serialize configs to JSON.
	configBytes, err := json.Marshal(config)
	if err != nil {
		return resp, fmt.Errorf("marshal container config: %w", err)
	}
	hostConfigBytes, err := json.Marshal(hostConfig)
	if err != nil {
		return resp, fmt.Errorf("marshal host config: %w", err)
	}
	networkingConfigBytes, err := json.Marshal(networkingConfig)
	if err != nil {
		return resp, fmt.Errorf("marshal networking config: %w", err)
	}
	platformBytes, err := json.Marshal(platform)
	if err != nil {
		return resp, fmt.Errorf("marshal platform: %w", err)
	}

	grpcResp, err := c.grpcClient.CreateContainer(ctx, &pb.CreateContainerRequest{
		Config:        configBytes,
		HostConfig:    hostConfigBytes,
		NetworkConfig: networkingConfigBytes,
		Platform:      platformBytes,
		Name:          name,
	})
	if err != nil {
		return resp, err
	}

	if err = json.Unmarshal(grpcResp.Response, &resp); err != nil {
		return resp, fmt.Errorf("unmarshal gRPC response: %w", err)
	}
	return resp, nil
}

// StartContainer starts a container with the given ID and options.
func (c *Client) StartContainer(ctx context.Context, id string, options container.StartOptions) error {
	optionsBytes, err := json.Marshal(options)
	if err != nil {
		return fmt.Errorf("marshal start options: %w", err)
	}

	_, err = c.grpcClient.StartContainer(ctx, &pb.StartContainerRequest{
		Id:      id,
		Options: optionsBytes,
	})
	return err
}
