package docker

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"io"
	"uncloud/internal/machine/api/pb"
)

// Server implements the gRPC Docker service that proxies requests to the Docker daemon.
type Server struct {
	pb.UnimplementedDockerServer
	client *client.Client
}

// NewServer creates a new Docker gRPC server with the provided Docker client.
func NewServer(cli *client.Client) *Server {
	return &Server{client: cli}
}

// CreateContainer creates a new container based on the given configuration.
func (s *Server) CreateContainer(ctx context.Context, req *pb.CreateContainerRequest) (*pb.CreateContainerResponse, error) {
	var config container.Config
	var hostConfig container.HostConfig
	var networkConfig network.NetworkingConfig
	var platform ocispec.Platform

	// Unmarshal configurations from the request.
	if err := json.Unmarshal(req.Config, &config); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unmarshal container config: %v", err)
	}
	if err := json.Unmarshal(req.HostConfig, &hostConfig); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unmarshal host config: %v", err)
	}
	if err := json.Unmarshal(req.NetworkConfig, &networkConfig); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unmarshal network config: %v", err)
	}
	if err := json.Unmarshal(req.Platform, &platform); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unmarshal platform: %v", err)
	}

	resp, err := s.client.ContainerCreate(ctx, &config, &hostConfig, &networkConfig, &platform, req.Name)
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "create container: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "create container: %v", err)
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal response: %v", err)
	}

	return &pb.CreateContainerResponse{Response: respBytes}, nil
}

// StartContainer starts a container with the given ID and options.
func (s *Server) StartContainer(ctx context.Context, req *pb.StartContainerRequest) (*emptypb.Empty, error) {
	var opts container.StartOptions
	if len(req.Options) > 0 {
		if err := json.Unmarshal(req.Options, &opts); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
		}
	}

	if err := s.client.ContainerStart(ctx, req.Id, opts); err != nil {
		return nil, status.Errorf(codes.Internal, "start container: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *Server) ListContainers(ctx context.Context, req *pb.ListContainersRequest) (*pb.ListContainersResponse, error) {
	var opts container.ListOptions
	if len(req.Options) > 0 {
		if err := json.Unmarshal(req.Options, &opts); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
		}
	}

	containers, err := s.client.ContainerList(ctx, opts)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list container: %v", err)
	}

	containersBytes, err := json.Marshal(containers)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal containers: %v", err)
	}

	return &pb.ListContainersResponse{
		Containers: containersBytes,
	}, nil
}

// RemoveContainer stops (kills after grace period) and removes a container with the given ID.
func (s *Server) RemoveContainer(ctx context.Context, req *pb.RemoveContainerRequest) (*emptypb.Empty, error) {
	var opts container.RemoveOptions
	if len(req.Options) > 0 {
		if err := json.Unmarshal(req.Options, &opts); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
		}
	}

	if err := s.client.ContainerRemove(ctx, req.Id, opts); err != nil {
		return nil, status.Errorf(codes.Internal, "remove container: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *Server) PullImage(req *pb.PullImageRequest, stream grpc.ServerStreamingServer[pb.JSONMessage]) error {
	ctx := stream.Context()

	// TODO: replace with another JSON serializable type (PullOptions.PrivilegeFunc is not serializable).
	var opts image.PullOptions
	if len(req.Options) > 0 {
		if err := json.Unmarshal(req.Options, &opts); err != nil {
			return status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
		}
	}

	respBody, err := s.client.ImagePull(ctx, req.Image, opts)
	if err != nil {
		return status.Errorf(codes.Internal, "pull image: %v", err)
	}
	defer respBody.Close()

	decoder := json.NewDecoder(respBody)
	errCh := make(chan error, 1)

	go func() {
		var raw json.RawMessage
		for {
			if err = decoder.Decode(&raw); err != nil {
				if errors.Is(err, io.EOF) {
					errCh <- nil
					return
				}
				errCh <- status.Errorf(codes.Internal, "decode image pull message: %v", err)
				return
			}

			if err = stream.Send(&pb.JSONMessage{Message: raw}); err != nil {
				errCh <- status.Errorf(codes.Internal, "send image pull message to stream: %v", err)
				return
			}
		}
	}()

	for {
		select {
		case err = <-errCh:
			return err
		case <-ctx.Done():
			return status.Errorf(codes.Canceled, "pull image: %v", ctx.Err())
		}
	}

}
