package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/jsonmessage"
	regtypes "github.com/google/go-containerregistry/pkg/v1/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/psviderski/uncloud/internal/docker"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Client is a gRPC client for the Docker service that provides a similar interface to the Docker HTTP client.
// TODO: it doesn't seem there is much value in having this intermediate Docker client.
// Consider merging it into the main pkg/client.
type Client struct {
	conn       *grpc.ClientConn
	GRPCClient pb.DockerClient
}

// NewClient creates a new Docker gRPC client with the provided gRPC connection.
func NewClient(conn *grpc.ClientConn) *Client {
	return &Client{
		conn:       conn,
		GRPCClient: pb.NewDockerClient(conn),
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

	grpcResp, err := c.GRPCClient.CreateContainer(ctx, &pb.CreateContainerRequest{
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
func (c *Client) InspectContainer(ctx context.Context, id string) (container.InspectResponse, error) {
	var resp container.InspectResponse

	grpcResp, err := c.GRPCClient.InspectContainer(ctx, &pb.InspectContainerRequest{Id: id})
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

	_, err = c.GRPCClient.StartContainer(ctx, &pb.StartContainerRequest{
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

	_, err = c.GRPCClient.StopContainer(ctx, &pb.StopContainerRequest{
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
	Containers []container.InspectResponse
}

func (c *Client) ListContainers(ctx context.Context, opts container.ListOptions) ([]MachineContainers, error) {
	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("marshal options: %w", err)
	}

	resp, err := c.GRPCClient.ListContainers(ctx, &pb.ListContainersRequest{Options: optsBytes})
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

	_, err = c.GRPCClient.RemoveContainer(ctx, &pb.RemoveContainerRequest{
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

// PullOptions defines the options for pulling an image from a remote registry.
// This is a copy of image.PullOptions from the Docker API without the PrivilegeFunc field that is non-serialisable.
type PullOptions struct {
	All bool
	// RegistryAuth is the base64 encoded credentials for the registry.
	RegistryAuth string
	Platform     string
}

func (c *Client) PullImage(ctx context.Context, image string, opts PullOptions) (<-chan docker.PullPushImageMessage, error) {
	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("marshal options: %w", err)
	}

	stream, err := c.GRPCClient.PullImage(ctx, &pb.PullImageRequest{Image: image, Options: optsBytes})
	if err != nil {
		return nil, err
	}

	ch := make(chan docker.PullPushImageMessage)

	go func() {
		defer close(ch)

		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- docker.PullPushImageMessage{Err: err}
				return
			}

			var jm jsonmessage.JSONMessage
			if err = json.Unmarshal(msg.Message, &jm); err != nil {
				ch <- docker.PullPushImageMessage{Err: fmt.Errorf("unmarshal JSON message: %w", err)}
				return
			}
			ch <- docker.PullPushImageMessage{Message: jm}
		}
	}()

	return ch, nil
}

// InspectImage returns the image information for the given image ID. The request may be sent to multiple machines.
func (c *Client) InspectImage(ctx context.Context, id string) ([]api.MachineImage, error) {
	resp, err := c.GRPCClient.InspectImage(ctx, &pb.InspectImageRequest{Id: id})
	if err != nil {
		// If the request was sent to only one machine, err is an actual error from the machine.
		if status.Convert(err).Code() == codes.NotFound {
			return nil, errdefs.NotFound(err)
		}
		return nil, err
	}

	notFoundCount := 0
	for _, msg := range resp.Messages {
		if msg.Metadata != nil && msg.Metadata.Status != nil && codes.Code(msg.Metadata.Status.Code) == codes.NotFound {
			notFoundCount++
		}
	}
	if len(resp.Messages) == notFoundCount {
		return nil, errdefs.NotFound(fmt.Errorf("image not found: %s", id))
	}

	images := make([]api.MachineImage, len(resp.Messages))
	for i, msg := range resp.Messages {
		images[i].Metadata = msg.Metadata
		if msg.Metadata != nil && msg.Metadata.Error != "" {
			continue
		}

		if err = json.Unmarshal(msg.Image, &images[i].Image); err != nil {
			return nil, fmt.Errorf("unmarshal image: %w", err)
		}
	}

	return images, nil
}

// InspectRemoteImage returns the image metadata for an image in a remote registry using the machine's Docker auth
// credentials if necessary. If the response from a machine doesn't contain an error, the api.RemoteImage will either
// contain an IndexManifest or an ImageManifest.
func (c *Client) InspectRemoteImage(ctx context.Context, id string) ([]api.MachineRemoteImage, error) {
	resp, err := c.GRPCClient.InspectRemoteImage(ctx, &pb.InspectRemoteImageRequest{Id: id})
	if err != nil {
		return nil, err
	}

	images := make([]api.MachineRemoteImage, 0, len(resp.Messages))
	var parseErr error
	for _, msg := range resp.Messages {
		mri, err := parseRemoteImageMessage(msg)
		if err != nil {
			parseErr = errors.Join(parseErr, err)
			continue
		}
		images = append(images, mri)
	}

	return images, parseErr
}

type manifestMediaType struct {
	MediaType regtypes.MediaType `json:"mediaType"`
}

func parseRemoteImageMessage(msg *pb.RemoteImage) (api.MachineRemoteImage, error) {
	mri := api.MachineRemoteImage{
		Metadata: msg.Metadata,
	}
	if msg.Metadata != nil && msg.Metadata.Error != "" {
		return mri, nil
	}

	ref, err := reference.ParseNormalizedNamed(msg.Reference)
	if err != nil {
		// The reference is the string representation of a valid canonical reference from the server which
		// is expected to be parseable.
		return mri, fmt.Errorf("parse reference: %w", err)
	}
	if canonicalRef, ok := ref.(reference.Canonical); ok {
		mri.Image.Reference = canonicalRef
	} else {
		return mri, fmt.Errorf("unexpected non-canonical image reference: %s", msg.Reference)
	}

	manifest := manifestMediaType{}
	if err = json.Unmarshal(msg.Manifest, &manifest); err != nil {
		return mri, fmt.Errorf("unmarshal manifest: %w", err)
	}

	switch {
	case manifest.MediaType.IsIndex():
		index := &ocispec.Index{}
		if err = json.Unmarshal(msg.Manifest, index); err != nil {
			return mri, fmt.Errorf("unmarshal index manifest: %w", err)
		}
		mri.Image.IndexManifest = index
	case manifest.MediaType.IsImage():
		image := &ocispec.Manifest{}
		if err = json.Unmarshal(msg.Manifest, image); err != nil {
			return mri, fmt.Errorf("unmarshal image manifest: %w", err)
		}
		mri.Image.ImageManifest = image
	default:
		return mri, fmt.Errorf("unexpected manifest media type: %s", manifest.MediaType)
	}

	return mri, nil
}

// CreateVolume creates a new volume with the given options.
func (c *Client) CreateVolume(ctx context.Context, opts volume.CreateOptions) (volume.Volume, error) {
	var vol volume.Volume

	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return vol, fmt.Errorf("marshal options: %w", err)
	}

	resp, err := c.GRPCClient.CreateVolume(ctx, &pb.CreateVolumeRequest{Options: optsBytes})
	if err != nil {
		return vol, err
	}

	if err = json.Unmarshal(resp.Volume, &vol); err != nil {
		return vol, fmt.Errorf("unmarshal volume: %w", err)
	}

	return vol, nil
}

type MachineVolumes struct {
	Metadata *pb.Metadata
	Response volume.ListResponse
}

func (c *Client) ListVolumes(ctx context.Context, opts volume.ListOptions) ([]MachineVolumes, error) {
	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("marshal options: %w", err)
	}

	resp, err := c.GRPCClient.ListVolumes(ctx, &pb.ListVolumesRequest{Options: optsBytes})
	if err != nil {
		return nil, err
	}

	machineVolumes := make([]MachineVolumes, len(resp.Messages))
	for i, msg := range resp.Messages {
		machineVolumes[i].Metadata = msg.Metadata
		if msg.Metadata != nil && msg.Metadata.Error != "" {
			continue
		}

		if err = json.Unmarshal(msg.Response, &machineVolumes[i].Response); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return machineVolumes, nil
}

// RemoveVolume removes a volume with the given ID.
func (c *Client) RemoveVolume(ctx context.Context, id string, force bool) error {
	_, err := c.GRPCClient.RemoveVolume(ctx, &pb.RemoveVolumeRequest{
		Id:    id,
		Force: force,
	})
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			return errdefs.NotFound(err)
		}
	}

	return err
}

// CreateServiceContainer creates a new container for the service with the given specifications.
func (c *Client) CreateServiceContainer(
	ctx context.Context, serviceID string, spec api.ServiceSpec, containerName string,
) (container.CreateResponse, error) {
	var resp container.CreateResponse

	specBytes, err := json.Marshal(spec)
	if err != nil {
		return resp, fmt.Errorf("marshal service spec: %w", err)
	}
	grpcResp, err := c.GRPCClient.CreateServiceContainer(ctx, &pb.CreateServiceContainerRequest{
		ServiceId:     serviceID,
		ServiceSpec:   specBytes,
		ContainerName: containerName,
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

// InspectServiceContainer returns the container information and service specification that was used to create the
// container with the given ID.
func (c *Client) InspectServiceContainer(ctx context.Context, id string) (api.ServiceContainer, error) {
	var resp api.ServiceContainer

	grpcResp, err := c.GRPCClient.InspectServiceContainer(ctx, &pb.InspectContainerRequest{Id: id})
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			return resp, errdefs.NotFound(err)
		}
		return resp, err
	}

	if err = json.Unmarshal(grpcResp.Container, &resp.Container); err != nil {
		return resp, fmt.Errorf("unmarshal container: %w", err)
	}
	if err = json.Unmarshal(grpcResp.ServiceSpec, &resp.ServiceSpec); err != nil {
		return resp, fmt.Errorf("unmarshal service spec: %w", err)
	}

	return resp, nil
}

type MachineServiceContainers struct {
	Metadata   *pb.Metadata
	Containers []api.ServiceContainer
}

// ListServiceContainers returns all containers on requested machines that belong to the service with the given
// name or ID. If serviceNameOrID is empty, all service containers are returned.
func (c *Client) ListServiceContainers(
	ctx context.Context, serviceNameOrID string, opts container.ListOptions,
) ([]MachineServiceContainers, error) {
	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("marshal options: %w", err)
	}

	resp, err := c.GRPCClient.ListServiceContainers(ctx, &pb.ListServiceContainersRequest{
		ServiceId: serviceNameOrID,
		Options:   optsBytes,
	})
	if err != nil {
		return nil, err
	}

	machineContainers := make([]MachineServiceContainers, len(resp.Messages))
	for i, msg := range resp.Messages {
		machineContainers[i].Metadata = msg.Metadata
		if msg.Metadata != nil && msg.Metadata.Error != "" {
			continue
		}

		containers := make([]api.ServiceContainer, len(msg.Containers))
		for j, sc := range msg.Containers {
			if err = json.Unmarshal(sc.Container, &containers[j].Container); err != nil {
				return nil, fmt.Errorf("unmarshal container: %w", err)
			}
			if err = json.Unmarshal(sc.ServiceSpec, &containers[j].ServiceSpec); err != nil {
				return nil, fmt.Errorf("unmarshal service spec: %w", err)
			}
		}

		machineContainers[i].Containers = containers
	}

	return machineContainers, nil
}

// RemoveServiceContainer stops (kills after grace period) and removes a service container with the given ID.
// A service container is a container that has been created with CreateServiceContainer.
func (c *Client) RemoveServiceContainer(ctx context.Context, id string, opts container.RemoveOptions) error {
	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("marshal options: %w", err)
	}

	_, err = c.GRPCClient.RemoveServiceContainer(ctx, &pb.RemoveContainerRequest{
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
