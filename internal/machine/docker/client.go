package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/jsonmessage"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
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
		if status.Convert(err).Code() == codes.NotFound {
			return resp, errdefs.NotFound(err)
		}
		return resp, err
	}

	if err = json.Unmarshal(grpcResp.Response, &resp); err != nil {
		return resp, fmt.Errorf("unmarshal gRPC response: %w", err)
	}
	return resp, nil
}

// InspectContainer returns the container information for the given container ID.
func (c *Client) InspectContainer(ctx context.Context, id string) (types.ContainerJSON, error) {
	var resp types.ContainerJSON

	grpcResp, err := c.grpcClient.InspectContainer(ctx, &pb.InspectContainerRequest{Id: id})
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			return resp, errdefs.NotFound(err)
		}
		return resp, err
	}

	if err = json.Unmarshal(grpcResp.Response, &resp); err != nil {
		return resp, fmt.Errorf("unmarshal gRPC response: %w", err)
	}
	return resp, nil
}

// StartContainer starts a container with the given ID and options.
func (c *Client) StartContainer(ctx context.Context, id string, opts container.StartOptions) error {
	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("marshal options: %w", err)
	}

	_, err = c.grpcClient.StartContainer(ctx, &pb.StartContainerRequest{
		Id:      id,
		Options: optsBytes,
	})
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			return errdefs.NotFound(err)
		}
	}
	return err
}

// StopContainer stops a container with the given ID and options.
func (c *Client) StopContainer(ctx context.Context, id string, opts container.StopOptions) error {
	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("marshal options: %w", err)
	}

	_, err = c.grpcClient.StopContainer(ctx, &pb.StopContainerRequest{
		Id:      id,
		Options: optsBytes,
	})
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			return errdefs.NotFound(err)
		}
	}
	return err
}

type MachineContainers struct {
	Metadata   *pb.Metadata
	Containers []types.ContainerJSON
}

func (c *Client) ListContainers(ctx context.Context, opts container.ListOptions) ([]MachineContainers, error) {
	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("marshal options: %w", err)
	}

	resp, err := c.grpcClient.ListContainers(ctx, &pb.ListContainersRequest{Options: optsBytes})
	if err != nil {
		return nil, err
	}

	machineContainers := make([]MachineContainers, len(resp.Messages))
	for i, msg := range resp.Messages {
		machineContainers[i].Metadata = msg.Metadata
		if msg.Metadata != nil && msg.Metadata.Error != "" {
			continue
		}

		if err = json.Unmarshal(msg.Containers, &machineContainers[i].Containers); err != nil {
			return nil, fmt.Errorf("unmarshal containers: %w", err)
		}
	}

	return machineContainers, nil
}

// RemoveContainer stops (kills after grace period) and removes a container with the given ID.
func (c *Client) RemoveContainer(ctx context.Context, id string, opts container.RemoveOptions) error {
	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("marshal options: %w", err)
	}

	_, err = c.grpcClient.RemoveContainer(ctx, &pb.RemoveContainerRequest{
		Id:      id,
		Options: optsBytes,
	})
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			return errdefs.NotFound(err)
		}
	}
	return err
}

type PullImageMessage struct {
	Message jsonmessage.JSONMessage
	Err     error
}

func (c *Client) PullImage(ctx context.Context, image string) (<-chan PullImageMessage, error) {
	stream, err := c.grpcClient.PullImage(ctx, &pb.PullImageRequest{Image: image})
	if err != nil {
		return nil, err
	}

	ch := make(chan PullImageMessage)

	go func() {
		defer close(ch)

		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- PullImageMessage{Err: err}
				return
			}

			var jm jsonmessage.JSONMessage
			if err = json.Unmarshal(msg.Message, &jm); err != nil {
				ch <- PullImageMessage{Err: fmt.Errorf("unmarshal JSON message: %w", err)}
				return
			}
			ch <- PullImageMessage{Message: jm}
		}
	}()

	return ch, nil
}

// ImageInspect returns the image information for the given image ID.
func (c *Client) ImageInspect(ctx context.Context, id string) (types.ImageInspect, error) {
	var resp types.ImageInspect

	grpcResp, err := c.grpcClient.InspectImage(ctx, &pb.InspectImageRequest{id: id})
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			return resp, errdefs.NotFound(err)
		}
		return resp, err
	}

	if err = json.Unmarshal(grpcResp.Response, &resp); err != nil {
		return resp, fmt.Errorf("unmarshal gRPC response: %w", err)
	}
	return resp, nil
}
