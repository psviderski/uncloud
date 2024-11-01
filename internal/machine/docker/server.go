package docker

import (
	"context"
	"encoding/json"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
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
	var options container.StartOptions
	if len(req.Options) > 0 {
		if err := json.Unmarshal(req.Options, &options); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal start options: %v", err)
		}
	}

	if err := s.client.ContainerStart(ctx, req.Id, options); err != nil {
		return nil, status.Errorf(codes.Internal, "start container: %v", err)
	}

	return &emptypb.Empty{}, nil
}
