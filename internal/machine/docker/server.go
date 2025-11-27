package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	dockercommand "github.com/docker/cli/cli/command"
	dockerconfig "github.com/docker/cli/cli/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/docker/go-units"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/jmoiron/sqlx"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/dns"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var fullDockerIDRegex = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Server implements the gRPC Docker service that proxies requests to the Docker daemon.
type Server struct {
	pb.UnimplementedDockerServer
	client  *client.Client
	service *Service
	db      *sqlx.DB
	// internalDNSIP is a function that returns the IP address of the internal DNS server. It may return an empty
	// address if the address is unknown (e.g. when the machine is not initialised yet).
	internalDNSIP func() netip.Addr
	// machineID is a function that returns the machine ID. It may return an empty string if the machine
	// is not initialised yet.
	machineID func() string
	// networkReady is a function that returns true if the Docker network is ready for containers.
	networkReady func() bool
	// waitForNetworkReady is a function that waits for the Docker network to be ready for containers.
	waitForNetworkReady func(ctx context.Context) error
}

type ServerOptions struct {
	// TODO: verify if we still need the network readiness checks as the cluster controller ensures the network
	//  is ready before starting the network API server. It may still be needed when communicating with the local
	//  API server but in this case we should probably fail until the cluster is initialised.
	NetworkReady        func() bool
	WaitForNetworkReady func(ctx context.Context) error
}

// NewServer creates a new Docker gRPC server with the provided Docker service.
func NewServer(service *Service, db *sqlx.DB, internalDNSIP func() netip.Addr, machineID func() string, opts ServerOptions) *Server {
	s := &Server{
		client:        service.Client,
		service:       service,
		db:            db,
		internalDNSIP: internalDNSIP,
		machineID:     machineID,
	}

	s.networkReady = opts.NetworkReady
	s.waitForNetworkReady = opts.WaitForNetworkReady

	return s
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
		if errdefs.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal response: %v", err)
	}

	return &pb.CreateContainerResponse{Response: respBytes}, nil
}

// InspectContainer returns the container information for the given container ID.
func (s *Server) InspectContainer(ctx context.Context, req *pb.InspectContainerRequest) (*pb.InspectContainerResponse, error) {
	resp, err := s.client.ContainerInspect(ctx, req.Id)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal response: %v", err)
	}

	return &pb.InspectContainerResponse{Response: respBytes}, nil
}

// StartContainer starts a container with the given ID and options.
func (s *Server) StartContainer(ctx context.Context, req *pb.StartContainerRequest) (*emptypb.Empty, error) {
	// Wait for Docker network to be ready before starting the container
	if s.waitForNetworkReady != nil {
		if err := s.waitForNetworkReady(ctx); err != nil {
			return nil, status.Errorf(codes.Unavailable, "Docker network not ready: %v", err)
		}
	} else if s.networkReady != nil && !s.networkReady() {
		return nil, status.Errorf(codes.Unavailable, "Docker network not ready")
	}

	var opts container.StartOptions
	if len(req.Options) > 0 {
		if err := json.Unmarshal(req.Options, &opts); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
		}
	}

	if err := s.client.ContainerStart(ctx, req.Id, opts); err != nil {
		if errdefs.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &emptypb.Empty{}, nil
}

// StopContainer stops a container with the given ID and options.
func (s *Server) StopContainer(ctx context.Context, req *pb.StopContainerRequest) (*emptypb.Empty, error) {
	var opts container.StopOptions
	if len(req.Options) > 0 {
		if err := json.Unmarshal(req.Options, &opts); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
		}
	}

	if err := s.client.ContainerStop(ctx, req.Id, opts); err != nil {
		if errdefs.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &emptypb.Empty{}, nil
}

func (s *Server) ListContainers(ctx context.Context, req *pb.ListContainersRequest) (*pb.ListContainersResponse, error) {
	var opts container.ListOptions
	if len(req.Options) > 0 {
		if err := json.Unmarshal(req.Options, &opts); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
		}

		// Handle filters separately because they implement custom JSON unmarshalling.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(req.Options, &raw); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options to raw map: %v", err)
		}

		if filtersBytes, ok := raw["Filters"]; ok {
			args, err := filters.FromJSON(string(filtersBytes))
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "unmarshal filters: %v", err)
			}
			opts.Filters = args
		}
	}

	containerSummaries, err := s.client.ContainerList(ctx, opts)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	containers := make([]container.InspectResponse, 0, len(containerSummaries))
	for _, cs := range containerSummaries {
		c, err := s.client.ContainerInspect(ctx, cs.ID)
		if err != nil {
			if errdefs.IsNotFound(err) {
				// The listed container may have been removed while we were inspecting other containers.
				continue
			}
			return nil, status.Errorf(codes.Internal, "inspect container %s: %v", cs.ID, err)
		}
		containers = append(containers, c)
	}

	containersBytes, err := json.Marshal(containers)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal containers: %v", err)
	}

	return &pb.ListContainersResponse{
		Messages: []*pb.MachineContainers{
			{
				Containers: containersBytes,
			},
		},
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
		if errdefs.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &emptypb.Empty{}, nil
}

func (s *Server) PullImage(req *pb.PullImageRequest, stream grpc.ServerStreamingServer[pb.JSONMessage]) error {
	ctx := stream.Context()

	var opts image.PullOptions
	if len(req.Options) > 0 {
		if err := json.Unmarshal(req.Options, &opts); err != nil {
			return status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
		}
	}

	if opts.RegistryAuth == "" {
		// Try to retrieve the authentication token for the image from the default local Docker config file.
		dockerConfig := dockerconfig.LoadDefaultConfigFile(os.Stderr)
		if encodedAuth, err := dockercommand.RetrieveAuthTokenFromImage(dockerConfig, req.Image); err == nil {
			opts.RegistryAuth = encodedAuth
		}
	}

	respBody, err := s.client.ImagePull(ctx, req.Image, opts)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
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
			return status.Error(codes.Canceled, ctx.Err().Error())
		}
	}
}

// InspectImage returns the image information for the given image ID.
func (s *Server) InspectImage(ctx context.Context, req *pb.InspectImageRequest) (*pb.InspectImageResponse, error) {
	resp, err := s.client.ImageInspect(ctx, req.Id)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal response: %v", err)
	}

	return &pb.InspectImageResponse{
		Messages: []*pb.Image{
			{
				Image: respBytes,
			},
		},
	}, nil
}

// InspectRemoteImage returns the image metadata for an image in a remote registry using the machine's Docker auth
// credentials if necessary.
func (s *Server) InspectRemoteImage(
	_ context.Context, req *pb.InspectRemoteImageRequest,
) (*pb.InspectRemoteImageResponse, error) {
	ref, err := name.ParseReference(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse image: %v", err)
	}

	desc, err := remote.Get(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "fetch image manifest: %v", err)
	}

	namedRef, err := reference.ParseNormalizedNamed(ref.String())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse image: %v", err)
	}

	var canonicalRef reference.Canonical
	if _, ok := namedRef.(reference.Canonical); ok {
		canonicalRef = namedRef.(reference.Canonical)
	} else {
		if canonicalRef, err = reference.WithDigest(namedRef, digest.Digest(desc.Digest.String())); err != nil {
			return nil, status.Errorf(codes.Internal, "add digest to image: %v", err)
		}
	}

	return &pb.InspectRemoteImageResponse{
		Messages: []*pb.RemoteImage{
			{
				Reference: reference.FamiliarString(canonicalRef),
				Manifest:  desc.Manifest,
			},
		},
	}, nil
}

// ListImages returns a list of all images matching the filter and indicates whether Docker is using the containerd
// image store.
func (s *Server) ListImages(ctx context.Context, req *pb.ListImagesRequest) (*pb.ListImagesResponse, error) {
	var opts image.ListOptions
	if len(req.Options) > 0 {
		if err := json.Unmarshal(req.Options, &opts); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
		}

		// Handle filters separately because they implement custom JSON unmarshalling.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(req.Options, &raw); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options to raw map: %v", err)
		}

		if filtersBytes, ok := raw["Filters"]; ok {
			args, err := filters.FromJSON(string(filtersBytes))
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "unmarshal filters: %v", err)
			}
			opts.Filters = args
		}
	}

	images, err := s.service.ListImages(ctx, opts)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	machineImages := pb.MachineImages{
		ContainerdStore: images.ContainerdStore,
	}
	if len(images.Images) > 0 {
		if machineImages.Images, err = json.Marshal(images.Images); err != nil {
			return nil, status.Errorf(codes.Internal, "marshal Docker images: %v", err)
		}
	}

	return &pb.ListImagesResponse{
		Messages: []*pb.MachineImages{&machineImages},
	}, nil
}

// RemoveImage removes an image with the given ID.
func (s *Server) RemoveImage(ctx context.Context, req *pb.RemoveImageRequest) (*pb.RemoveImageResponse, error) {
	var opts image.RemoveOptions
	if len(req.Options) > 0 {
		if err := json.Unmarshal(req.Options, &opts); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
		}
	}

	deleteResp, err := s.client.ImageRemove(ctx, req.Image, opts)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	respBytes, err := json.Marshal(deleteResp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal response: %v", err)
	}

	return &pb.RemoveImageResponse{
		Messages: []*pb.MachineRemoveImageResponse{
			{
				Response: respBytes,
			},
		},
	}, nil
}

// PruneImages prunes unused images.
func (s *Server) PruneImages(ctx context.Context, req *pb.PruneImagesRequest) (*pb.PruneImagesResponse, error) {
	var filtersArgs filters.Args
	if len(req.Filters) > 0 {
		var err error
		filtersArgs, err = filters.FromJSON(string(req.Filters))
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal filters: %v", err)
		}
	}

	report, err := s.client.ImagesPrune(ctx, filtersArgs)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	reportBytes, err := json.Marshal(report)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal report: %v", err)
	}

	return &pb.PruneImagesResponse{
		Messages: []*pb.MachinePruneImagesResponse{
			{
				Report: reportBytes,
			},
		},
	}, nil
}

// CreateVolume creates a new volume with the given options.
func (s *Server) CreateVolume(ctx context.Context, req *pb.CreateVolumeRequest) (*pb.CreateVolumeResponse, error) {
	var opts volume.CreateOptions
	if err := json.Unmarshal(req.Options, &opts); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
	}

	// Always add the managed label to the volume to indicate that it is managed by Uncloud.
	if opts.Labels == nil {
		opts.Labels = make(map[string]string)
	}
	opts.Labels[api.LabelManaged] = ""

	vol, err := s.client.VolumeCreate(ctx, opts)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	volBytes, err := json.Marshal(vol)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal volume: %v", err)
	}

	return &pb.CreateVolumeResponse{Volume: volBytes}, nil
}

// ListVolumes returns a list of all volumes matching the filter.
func (s *Server) ListVolumes(ctx context.Context, req *pb.ListVolumesRequest) (*pb.ListVolumesResponse, error) {
	var opts volume.ListOptions
	if len(req.Options) > 0 {
		if err := json.Unmarshal(req.Options, &opts); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
		}

		// Handle filters separately because they implement custom JSON unmarshalling.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(req.Options, &raw); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options to raw map: %v", err)
		}

		if filtersBytes, ok := raw["Filters"]; ok {
			args, err := filters.FromJSON(string(filtersBytes))
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "unmarshal filters: %v", err)
			}
			opts.Filters = args
		}
	}

	resp, err := s.client.VolumeList(ctx, opts)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal response: %v", err)
	}

	return &pb.ListVolumesResponse{
		Messages: []*pb.MachineVolumes{
			{
				Response: respBytes,
			},
		},
	}, nil
}

// RemoveVolume removes a volume with the given ID.
func (s *Server) RemoveVolume(ctx context.Context, req *pb.RemoveVolumeRequest) (*emptypb.Empty, error) {
	if err := s.client.VolumeRemove(ctx, req.Id, req.Force); err != nil {
		if errdefs.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &emptypb.Empty{}, nil
}

// CreateServiceContainer creates a new container for the service with the given specifications.
// TODO: move the main logic to the Docker service and remove db dependency from the server.
func (s *Server) CreateServiceContainer(
	ctx context.Context, req *pb.CreateServiceContainerRequest,
) (*pb.CreateContainerResponse, error) {
	if !api.ValidateServiceID(req.ServiceId) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service ID: '%s'", req.ServiceId)
	}

	var spec api.ServiceSpec
	if err := json.Unmarshal(req.ServiceSpec, &spec); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unmarshal service spec: %v", err)
	}
	spec = spec.SetDefaults()
	if err := spec.Validate(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service spec: %v", err)
	}

	containerName := req.ContainerName
	if containerName == "" {
		suffix, err := secret.RandomAlphaNumeric(4)
		if err != nil {
			return nil, fmt.Errorf("generate random suffix: %w", err)
		}
		containerName = fmt.Sprintf("%s-%s", spec.Name, suffix)
	}

	envVars := maps.Clone(spec.Container.Env)
	if envVars == nil {
		envVars = make(api.EnvVars)
	}

	// Inject the machine ID if available
	if s.machineID != nil {
		if machineID := s.machineID(); machineID != "" {
			envVars["UNCLOUD_MACHINE_ID"] = machineID
		}
	}

	config := &container.Config{
		Cmd:        spec.Container.Command,
		Env:        envVars.ToSlice(),
		Entrypoint: spec.Container.Entrypoint,
		Hostname:   containerName,
		Image:      spec.Container.Image,
		Labels: map[string]string{
			api.LabelServiceID:   req.ServiceId,
			api.LabelServiceName: spec.Name,
			api.LabelServiceMode: spec.Mode,
			api.LabelManaged:     "",
		},
		User: spec.Container.User,
	}
	if spec.Mode == "" {
		config.Labels[api.LabelServiceMode] = api.ServiceModeReplicated
	}
	if hc := spec.Container.Healthcheck; hc != nil {
		if hc.Disable {
			config.Healthcheck = &container.HealthConfig{
				Test: []string{"NONE"},
			}
		} else {
			config.Healthcheck = &container.HealthConfig{
				Test:          hc.Test,
				Interval:      hc.Interval,
				Timeout:       hc.Timeout,
				StartPeriod:   hc.StartPeriod,
				StartInterval: hc.StartInterval,
				Retries:       int(hc.Retries),
			}
		}
	}

	// TODO: do not set the ports as container labels once migrated to retrieve them from the spec in DB.
	var err error
	if len(spec.Ports) > 0 {
		encodedPorts := make([]string, len(spec.Ports))
		for i, p := range spec.Ports {
			encodedPorts[i], err = p.String()
			if err != nil {
				return nil, fmt.Errorf("encode service port spec: %w", err)
			}
		}

		config.Labels[api.LabelServicePorts] = strings.Join(encodedPorts, ",")
	}

	mounts, err := ToDockerMounts(spec.Volumes, spec.Container.VolumeMounts)
	if err != nil {
		return nil, err
	}
	if err = s.verifyDockerVolumesExist(ctx, mounts); err != nil {
		return nil, err
	}

	portBindings := make(nat.PortMap)
	for _, p := range spec.Ports {
		if p.Mode != api.PortModeHost {
			continue
		}
		port := nat.Port(fmt.Sprintf("%d/%s", p.ContainerPort, p.Protocol))
		portBindings[port] = []nat.PortBinding{
			{
				HostPort: strconv.Itoa(int(p.PublishedPort)),
			},
		}
		if p.HostIP.IsValid() {
			portBindings[port][0].HostIP = p.HostIP.String()
		}
	}
	hostConfig := &container.HostConfig{
		CapAdd:       spec.Container.CapAdd,
		CapDrop:      spec.Container.CapDrop,
		Binds:        spec.Container.Volumes,
		Init:         spec.Container.Init,
		Mounts:       mounts,
		PortBindings: portBindings,
		Privileged:   spec.Container.Privileged,
		Resources: container.Resources{
			NanoCPUs:          spec.Container.Resources.CPU,
			Memory:            spec.Container.Resources.Memory,
			MemoryReservation: spec.Container.Resources.MemoryReservation,
			Devices:           toDockerDevices(spec.Container.Resources.Devices),
			DeviceRequests:    spec.Container.Resources.DeviceReservations,
			Ulimits:           toDockerUlimits(spec.Container.Resources.Ulimits),
		},
		// Restart service containers if they exit or a machine restarts unless they are explicitly stopped.
		// For one-off containers and batch jobs we plan to use a different service type/mode.
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
		Sysctls: spec.Container.Sysctls,
	}

	// Configure the container to use the internal DNS server if it's available.
	dnsIP := s.internalDNSIP()
	if dnsIP.IsValid() {
		hostConfig.DNS = []string{dnsIP.String()}
		// Optimize DNS resolution for service discovery by appending the search domain to names without a dot.
		// For example, the first attempt for "my-service" will be "my-service.internal".
		hostConfig.DNSOptions = []string{"ndots:1"}
		hostConfig.DNSSearch = []string{dns.InternalDomain}
	}

	if spec.Container.LogDriver != nil {
		hostConfig.LogConfig = container.LogConfig{
			Type:   spec.Container.LogDriver.Name,
			Config: spec.Container.LogDriver.Options,
		}
	}

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			NetworkName: {},
		},
	}

	resp, err := s.client.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, containerName)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Inject configs into the created container
	if err = s.injectConfigs(ctx, resp.ID, spec.Configs, spec.Container.ConfigMounts); err != nil {
		// Remove the container if config injection fails
		_ = s.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{RemoveVolumes: true})
		return nil, status.Errorf(codes.Internal, "inject configs: %v", err)
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal response: %v", err)
	}

	// Store the container spec in the database or remove the container with its anonymous volumes if storing fails.
	removeContainer := func() {
		_ = s.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{RemoveVolumes: true})
	}

	specBytes, err := json.Marshal(spec)
	if err != nil {
		removeContainer()
		return nil, status.Errorf(codes.Internal, "marshal service spec: %v", err)
	}

	if _, err = s.db.ExecContext(ctx, `INSERT INTO containers (id, service_spec) VALUES ($1, $2)`,
		resp.ID, string(specBytes)); err != nil {
		removeContainer()
		return nil, status.Errorf(codes.Internal, "store container in database: %v", err)
	}

	return &pb.CreateContainerResponse{Response: respBytes}, nil
}

func ToDockerMounts(volumes []api.VolumeSpec, mounts []api.VolumeMount) ([]mount.Mount, error) {
	normalisedVolumes := make([]api.VolumeSpec, len(volumes))
	for i, v := range volumes {
		normalisedVolumes[i] = v.SetDefaults()
	}

	dockerMounts := make([]mount.Mount, 0, len(mounts))
	for _, m := range mounts {
		idx := slices.IndexFunc(normalisedVolumes, func(v api.VolumeSpec) bool {
			return v.Name == m.VolumeName
		})
		if idx == -1 {
			return nil, fmt.Errorf("volume mount references a volume that doesn't exist in the volumes spec: '%s'",
				m.VolumeName)
		}

		vol := normalisedVolumes[idx]
		if err := vol.Validate(); err != nil {
			return nil, fmt.Errorf("invalid volume: %w", err)
		}

		dm := mount.Mount{
			Type:     mount.Type(vol.Type),
			Target:   m.ContainerPath,
			ReadOnly: m.ReadOnly,
		}

		switch vol.Type {
		case api.VolumeTypeBind:
			dm.Source = vol.BindOptions.HostPath
			dm.BindOptions = toDockerBindOptions(vol.BindOptions)
		case api.VolumeTypeVolume:
			dm.Source = vol.DockerVolumeName()
			dm.VolumeOptions = &mount.VolumeOptions{
				NoCopy:       vol.VolumeOptions.NoCopy,
				Labels:       vol.VolumeOptions.Labels,
				Subpath:      vol.VolumeOptions.SubPath,
				DriverConfig: vol.VolumeOptions.Driver,
			}
		case api.VolumeTypeTmpfs:
			dm.TmpfsOptions = vol.TmpfsOptions
		default:
			return nil, fmt.Errorf("unsupported volume type: '%s'", vol.Type)
		}

		dockerMounts = append(dockerMounts, dm)
	}

	return dockerMounts, nil
}

// injectConfigs writes config content directly into the container.
// It processes ConfigSpecs and ConfigMounts to mount configuration content into the container filesystem.
func (s *Server) injectConfigs(ctx context.Context, containerID string, configs []api.ConfigSpec, mounts []api.ConfigMount) error {
	if len(configs) == 0 || len(mounts) == 0 {
		return nil
	}

	if err := api.ValidateConfigsAndMounts(configs, mounts); err != nil {
		return fmt.Errorf("validate configs and mounts: %w", err)
	}

	// Create a map of config name to config spec for quick lookup
	configMap := make(map[string]api.ConfigSpec)
	for _, config := range configs {
		configMap[config.Name] = config
	}

	// Process each config mount
	for _, m := range mounts {
		config, exists := configMap[m.ConfigName]
		if !exists {
			return fmt.Errorf("config mount references a config that doesn't exist: '%s'", m.ConfigName)
		}

		// Determine target path in container
		targetPath := m.ContainerPath
		if targetPath == "" {
			// This is the default from the Compose spec
			targetPath = filepath.Join("/", m.ConfigName)
		}

		// Determine file mode
		fileMode := os.FileMode(0o444) // Default permissions
		if m.Mode != nil {
			fileMode = *m.Mode
		}

		uid, err := m.GetNumericUid()
		if err != nil {
			return fmt.Errorf("invalid Uid: %w", err)
		}

		gid, err := m.GetNumericGid()
		if err != nil {
			return fmt.Errorf("invalid Gid: %w", err)
		}

		// Copy the config content directly into the container
		if err := s.copyContentToContainer(
			ctx, containerID, config.Content, targetPath, uid, gid, fileMode,
		); err != nil {
			return fmt.Errorf("copy config file '%s' to container: %w", config.Name, err)
		}

		slog.Debug("Injected config into container",
			"config", config.Name,
			"container", containerID[:12],
			"target", targetPath)
	}

	return nil
}

// copyContentToContainer copies content directly to a file in the container using Docker's CopyToContainer API.
// It will create any intermediate directories in the target path that don't exist.
func (s *Server) copyContentToContainer(ctx context.Context, containerID string, content []byte, targetPath string, uid *uint64, gid *uint64, fileMode os.FileMode) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Trim leading slash(es) to avoid double slashes in the tar path
	tarPath := strings.TrimPrefix(targetPath, "/")

	// Create tar header with full path
	header := &tar.Header{
		Name:     tarPath,
		Size:     int64(len(content)),
		Mode:     int64(fileMode),
		ModTime:  time.Now(),
		Typeflag: tar.TypeReg,
	}

	// Set ownership if specified
	if uid != nil {
		header.Uid = int(*uid)
	}
	if gid != nil {
		header.Gid = int(*gid)
	}

	// Write header and content to tar archive
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header: %w", err)
	}
	if _, err := tw.Write(content); err != nil {
		return fmt.Errorf("write content to tar: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar writer: %w", err)
	}

	// Always extract to root. The tar archive contains the full path, so tar will
	// automatically create any intermediate directories that don't exist in the container.
	if err := s.client.CopyToContainer(
		ctx,
		containerID,
		"/",
		&buf,
		container.CopyToContainerOptions{CopyUIDGID: true},
	); err != nil {
		return fmt.Errorf("copy to container: %w", err)
	}

	return nil
}

func toDockerBindOptions(opts *api.BindOptions) *mount.BindOptions {
	if opts == nil {
		return nil
	}

	dockerOpts := &mount.BindOptions{
		Propagation:      opts.Propagation,
		CreateMountpoint: opts.CreateHostPath,
	}

	switch opts.Recursive {
	case "disabled":
		dockerOpts.NonRecursive = true
	case "writable":
		dockerOpts.ReadOnlyNonRecursive = true
	case "readonly":
		dockerOpts.ReadOnlyForceRecursive = true
	}

	return dockerOpts
}

func toDockerUlimits(ulimits map[string]api.Ulimit) []*units.Ulimit {
	if len(ulimits) == 0 {
		return nil
	}

	dockerUlimits := make([]*units.Ulimit, 0, len(ulimits))
	for name, u := range ulimits {
		dockerUlimits = append(dockerUlimits, &units.Ulimit{
			Name: name,
			Soft: u.Soft,
			Hard: u.Hard,
		})
	}

	return dockerUlimits
}

func toDockerDevices(devices []api.DeviceMapping) []container.DeviceMapping {
	if len(devices) == 0 {
		return nil
	}

	dockerDevices := make([]container.DeviceMapping, 0, len(devices))
	for _, d := range devices {
		dockerDevices = append(dockerDevices, container.DeviceMapping{
			PathOnHost:        d.HostPath,
			PathInContainer:   d.ContainerPath,
			CgroupPermissions: d.CgroupPermissions,
		})
	}

	return dockerDevices
}

// verifyDockerVolumesExist checks if the Docker named volumes referenced in the mounts exist on the machine.
func (s *Server) verifyDockerVolumesExist(ctx context.Context, mounts []mount.Mount) error {
	for _, m := range mounts {
		if m.Type != mount.TypeVolume {
			continue
		}

		// TODO: non-local volume drivers should likely be handled differently (needs proper investigation).
		if _, err := s.client.VolumeInspect(ctx, m.Source); err != nil {
			if errdefs.IsNotFound(err) {
				return status.Errorf(codes.NotFound, "volume '%s' not found", m.Source)
			}
			return status.Errorf(codes.Internal, "inspect volume '%s': %v", m.Source, err.Error())
		}
		// TODO: check if the volume driver and options are the same as in the mount and fail if not.
		//  Should we even ignore driver-specific options in the volume spec for externally managed volumes?
		//  Instead, just inspect the existing volume and construct the mount from it.
	}

	return nil
}

// InspectServiceContainer returns the container information and service specification that was used to create the
// container with the given ID.
func (s *Server) InspectServiceContainer(
	ctx context.Context, req *pb.InspectContainerRequest,
) (*pb.ServiceContainer, error) {
	serviceCtr, err := s.service.InspectServiceContainer(ctx, req.Id)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	ctrBytes, err := json.Marshal(serviceCtr.Container)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal container: %v", err)
	}

	specBytes, err := json.Marshal(serviceCtr.ServiceSpec)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal service spec: %v", err)
	}

	return &pb.ServiceContainer{
		Container:   ctrBytes,
		ServiceSpec: specBytes,
	}, nil
}

// ListServiceContainers returns all containers that belong to the service with the given name or ID.
// If req.ServiceId is empty, all service containers are returned.
func (s *Server) ListServiceContainers(
	ctx context.Context, req *pb.ListServiceContainersRequest,
) (*pb.ListServiceContainersResponse, error) {
	var opts container.ListOptions
	if len(req.Options) > 0 {
		if err := json.Unmarshal(req.Options, &opts); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
		}

		// Handle filters separately because they implement custom JSON unmarshalling.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(req.Options, &raw); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unmarshal options to raw map: %v", err)
		}

		if filtersBytes, ok := raw["Filters"]; ok {
			args, err := filters.FromJSON(string(filtersBytes))
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "unmarshal filters: %v", err)
			}
			opts.Filters = args
		}
	}

	containers, err := s.service.ListServiceContainers(ctx, req.ServiceId, opts)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert to protobuf format.
	pbContainers := make([]*pb.ServiceContainer, 0, len(containers))
	for _, ctr := range containers {
		ctrBytes, err := json.Marshal(ctr.Container)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "marshal container: %v", err)
		}

		specBytes, err := json.Marshal(ctr.ServiceSpec)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "marshal service spec: %v", err)
		}

		pbContainers = append(pbContainers, &pb.ServiceContainer{
			Container:   ctrBytes,
			ServiceSpec: specBytes,
		})
	}

	return &pb.ListServiceContainersResponse{
		Messages: []*pb.MachineServiceContainers{
			{
				Containers: pbContainers,
			},
		},
	}, nil
}

// RemoveServiceContainer stops (kills after grace period) and removes a service container with the given ID.
// The difference between this method and RemoveContainer is that it also removes the container from the machine
// database.
func (s *Server) RemoveServiceContainer(ctx context.Context, req *pb.RemoveContainerRequest) (*emptypb.Empty, error) {
	ctrID := req.Id
	// If the ID is not a full Docker ID, inspect the container to get its full ID.
	if !fullDockerIDRegex.MatchString(req.Id) {
		ctr, err := s.client.ContainerInspect(ctx, req.Id)
		if err != nil {
			if errdefs.IsNotFound(err) {
				return nil, status.Error(codes.NotFound, err.Error())
			}
			return nil, status.Error(codes.Internal, err.Error())
		}
		ctrID = ctr.ID
	}

	resp, err := s.RemoveContainer(ctx, req)
	if err != nil {
		return nil, err
	}

	if _, err = s.db.ExecContext(ctx, `DELETE FROM containers WHERE id = $1`, ctrID); err != nil {
		slog.Error("Failed to remove container from machine database.", "err", err, "id", ctrID)
		// Do not return an error because the container has already been removed from the Docker daemon.
		// The orphaned db record will be ignored and eventually cleaned up by the garbage collector.
	}

	return resp, nil
}

// logsHeartbeatInterval is the interval at which heartbeat entries are sent when there are no logs to stream.
const logsHeartbeatInterval = 200 * time.Millisecond

// ContainerLogs streams logs from a container.
func (s *Server) ContainerLogs(
	req *pb.ContainerLogsRequest, stream grpc.ServerStreamingServer[pb.ContainerLogEntry],
) error {
	// Stream context is cancelled when the client has disconnected or the stream has ended.
	ctx := stream.Context()

	opts := ContainerLogsOptions{
		ContainerID: req.ContainerId,
		Follow:      req.Follow,
		Tail:        int(req.Tail),
		Since:       req.Since,
		Until:       req.Until,
	}

	logsCh, err := s.service.ContainerLogs(ctx, opts)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return status.Error(codes.NotFound, err.Error())
		}
		return status.Errorf(codes.Internal, "get container logs: %v", err)
	}

	log := slog.With("container_id", req.ContainerId, "stream_id", fmt.Sprintf("%p", stream)[2:])
	log.Debug("Starting container logs streaming.",
		"follow", req.Follow, "tail", req.Tail, "since", req.Since, "until", req.Until)

	// Heartbeats are needed only when following logs to let the client know when there are no new log entries
	// to allow it to advance the watermark of last received log timestamp.
	var heartbeatCh <-chan time.Time
	if req.Follow {
		heartbeatTicker := time.NewTicker(logsHeartbeatInterval)
		defer heartbeatTicker.Stop()
		heartbeatCh = heartbeatTicker.C
	}

	started := time.Now()
	lastSent := time.Time{}

	for {
		select {
		case entry, ok := <-logsCh:
			if !ok {
				// Channel closed, no more log entries.
				return nil
			}

			if entry.Err != nil {
				return status.Error(codes.Internal, entry.Err.Error())
			}

			pbEntry := &pb.ContainerLogEntry{
				Stream:    api.LogStreamTypeToProto(entry.Stream),
				Timestamp: timestamppb.New(entry.Timestamp),
				Message:   entry.Message,
			}
			if err = stream.Send(pbEntry); err != nil {
				return status.Errorf(codes.Internal, "send log entry: %v", err)
			}
			lastSent = entry.Timestamp

		case now := <-heartbeatCh:
			// Only send heartbeat if no log entries have been sent since the last heartbeat interval or
			// if no log entries have been sent at all for at least a heartbeat interval since starting.
			if now.Sub(lastSent) < logsHeartbeatInterval ||
				(lastSent.IsZero() && now.Sub(started) < logsHeartbeatInterval) {
				continue
			}

			// Use the timestamp one heartbeat in the past to be conservative. This reduces the chance of sending
			// a timestamp that is greater than a log entry currently being parsed but not yet sent, which would
			// cause the client to incorrectly believe it has received all logs up to that point.
			heartbeat := &pb.ContainerLogEntry{
				Stream:    pb.ContainerLogEntry_HEARTBEAT,
				Timestamp: timestamppb.New(now.Add(-logsHeartbeatInterval)),
			}
			if err = stream.Send(heartbeat); err != nil {
				return status.Errorf(codes.Internal, "send log stream heartbeat: %v", err)
			}
			lastSent = heartbeat.Timestamp.AsTime()
			log.Debug("Sent log stream heartbeat.", "timestamp", lastSent)

		case <-ctx.Done():
			return status.Error(codes.Canceled, ctx.Err().Error())
		}
	}
}

// receiveExecConfig receives and validates the initial exec configuration from the stream.
func (s *Server) receiveExecConfig(stream pb.Docker_ExecContainerServer) (*pb.ExecConfig, api.ExecOptions, error) {
	req, err := stream.Recv()
	if err != nil {
		return nil, api.ExecOptions{}, status.Errorf(codes.InvalidArgument, "receive config: %v", err)
	}

	execConfig := req.GetConfig()
	if execConfig == nil {
		return nil, api.ExecOptions{}, status.Error(codes.InvalidArgument, "first message must contain exec config")
	}

	// Unmarshal the Uncloud's execOpts
	var execOpts api.ExecOptions
	if err := json.Unmarshal(execConfig.Options, &execOpts); err != nil {
		return nil, api.ExecOptions{}, status.Errorf(codes.InvalidArgument, "unmarshal exec config: %v", err)
	}

	return execConfig, execOpts, nil
}

// handleServerExecInput reads from the gRPC stream and writes to Docker stdin, handling resize requests.
func (s *Server) handleServerExecInput(
	ctx context.Context,
	stream pb.Docker_ExecContainerServer,
	attachConn types.HijackedResponse,
	execID string,
	tty bool,
) error {
	slog.Debug("Input goroutine started", "exec_id", execID, "tty", tty)
	defer slog.Debug("Input goroutine exited", "exec_id", execID)

	defer attachConn.CloseWrite()
	for {
		select {
		case <-ctx.Done():
			slog.Debug("Input goroutine context canceled via context", "exec_id", execID)
			return nil
		default:
		}

		req, err := stream.Recv()
		switch {
		case errors.Is(err, io.EOF):
			slog.Debug("Input goroutine received EOF", "exec_id", execID)
			return nil
		case status.Code(err) == codes.Canceled:
			// Can be the case when the output goroutine ends and the stream context is canceled.
			slog.Debug("Input goroutine context canceled", "exec_id", execID)
			return nil
		case err == nil:
			// continue processing
		default:
			return fmt.Errorf("receive from stream: %w", err)
		}

		switch payload := req.Payload.(type) {
		case *pb.ExecContainerRequest_Stdin:
			if _, err := attachConn.Conn.Write(payload.Stdin); err != nil {
				return fmt.Errorf("write to stdin: %w", err)
			}
		case *pb.ExecContainerRequest_Resize:
			if tty {
				resizeOpts := container.ResizeOptions{
					Height: uint(payload.Resize.Height),
					Width:  uint(payload.Resize.Width),
				}
				if err := s.client.ContainerExecResize(ctx, execID, resizeOpts); err != nil {
					slog.Warn("Failed to resize TTY", "err", err, "exec_id", execID)
				}
			}
		}
	}
}

// handleServerExecOutput reads from Docker stdout/stderr and writes to the gRPC stream.
func (s *Server) handleServerExecOutput(
	stream pb.Docker_ExecContainerServer,
	attachResp types.HijackedResponse,
	execID string,
	tty bool,
) error {
	slog.Debug("Output goroutine started", "exec_id", execID, "tty", tty)
	defer slog.Debug("Output goroutine exited", "exec_id", execID)

	if tty {
		// In TTY mode, all output is stdout - copy directly to stream
		stdoutWriter := &grpcStreamWriter{stream: stream, isStderr: false}
		_, err := io.Copy(stdoutWriter, attachResp.Reader)
		if err != nil && err != io.EOF {
			return fmt.Errorf("copy tty output: %w", err)
		}
		return nil
	} else {
		// In non-TTY mode, Docker multiplexes stdout/stderr with headers
		// Use stdcopy to demultiplex
		slog.Debug("Starting StdCopy for non-TTY", "exec_id", execID)
		stdoutWriter := &grpcStreamWriter{stream: stream, isStderr: false}
		stderrWriter := &grpcStreamWriter{stream: stream, isStderr: true}

		written, err := stdcopy.StdCopy(stdoutWriter, stderrWriter, attachResp.Reader)
		slog.Debug("StdCopy completed", "exec_id", execID, "bytes", written, "err", err)
		if err != nil && err != io.EOF {
			return fmt.Errorf("demultiplex docker output: %w", err)
		}
		return nil
	}
}

// grpcStreamWriter is a writer that sends data to a gRPC stream as stdout or stderr.
type grpcStreamWriter struct {
	stream   pb.Docker_ExecContainerServer
	isStderr bool
}

func (w *grpcStreamWriter) Write(p []byte) (n int, err error) {
	data := make([]byte, len(p))
	copy(data, p)

	var resp *pb.ExecContainerResponse
	if w.isStderr {
		resp = &pb.ExecContainerResponse{
			Payload: &pb.ExecContainerResponse_Stderr{Stderr: data},
		}
	} else {
		resp = &pb.ExecContainerResponse{
			Payload: &pb.ExecContainerResponse_Stdout{Stdout: data},
		}
	}

	if err := w.stream.Send(resp); err != nil {
		return 0, err
	}
	return len(p), nil
}

// ExecContainer executes a command in a running container with bidirectional streaming for stdin/stdout/stderr.
func (s *Server) ExecContainer(stream pb.Docker_ExecContainerServer) error {
	slog.Debug("ExecContainer server-side called")
	defer slog.Debug("ExecContainer server-side ended")

	ctx := stream.Context()

	// Receive and validate configuration
	execConfig, execOpts, err := s.receiveExecConfig(stream)
	if err != nil {
		return err
	}

	// Convert to Docker's ExecOptions
	dockerExecOpts := container.ExecOptions{
		Cmd:          execOpts.Command,
		AttachStdin:  execOpts.AttachStdin,
		AttachStdout: execOpts.AttachStdout,
		AttachStderr: execOpts.AttachStderr,
		Tty:          execOpts.Tty,
		User:         execOpts.User,
		Privileged:   execOpts.Privileged,
		WorkingDir:   execOpts.WorkingDir,
		Env:          execOpts.Env,
	}

	// Create the exec instance
	execResp, err := s.client.ContainerExecCreate(ctx, execConfig.ContainerId, dockerExecOpts)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return status.Error(codes.NotFound, err.Error())
		}
		return status.Errorf(codes.Internal, "create exec: %v", err)
	}

	// Send the exec ID back to the client
	if err := stream.Send(&pb.ExecContainerResponse{
		Payload: &pb.ExecContainerResponse_ExecId{ExecId: execResp.ID},
	}); err != nil {
		return status.Errorf(codes.Internal, "send exec ID: %v", err)
	}
	slog.Debug("Sent exec ID to the client", "exec_id", execResp.ID)

	// For detached mode, start without attaching
	if execOpts.Detach {
		dockerStartOpts := container.ExecStartOptions{
			Tty:    dockerExecOpts.Tty,
			Detach: true,
		}
		if err := s.client.ContainerExecStart(ctx, execResp.ID, dockerStartOpts); err != nil {
			return status.Errorf(codes.Internal, "start exec: %v", err)
		}
		return nil
	}

	// For attached mode, attach to the exec instance
	attachOpts := container.ExecAttachOptions{
		Tty: dockerExecOpts.Tty,
	}
	attachConn, err := s.client.ContainerExecAttach(ctx, execResp.ID, attachOpts)
	if err != nil {
		return status.Errorf(codes.Internal, "attach to exec: %v", err)
	}
	defer attachConn.Close()

	// Create a cancelable context for the input handler
	handlerCtx, cancelInput := context.WithCancel(ctx)
	defer cancelInput()

	// Create a channel to wait for output completion
	outputDone := make(chan error, 1)

	// Start stdin handler if stdin is attached
	if dockerExecOpts.AttachStdin {
		go func() {
			err := s.handleServerExecInput(handlerCtx, stream, attachConn, execResp.ID, dockerExecOpts.Tty)
			if err != nil {
				slog.Warn("Error in exec input handler", "err", err, "exec_id", execResp.ID)
			}
		}()
	} else {
		// If not attaching stdin, close the write side immediately
		attachConn.CloseWrite()
	}

	// Start output handler
	// We only wait for this goroutine to complete - it signals when the exec process finishes
	go func() {
		outputDone <- s.handleServerExecOutput(stream, attachConn, execResp.ID, dockerExecOpts.Tty)
	}()

	// Wait for the output goroutine to complete (it signals when done)
	// We only wait for the output handler goroutine, not for the stdin one.
	if err := <-outputDone; err != nil {
		slog.Warn("Error in exec output handler", "err", err, "exec_id", execResp.ID)
	}
	// Do a best-effort cancellation of the stdin handler.
	// We can't guarantee immediate exit because it may be blocked on stream.Recv(), but at
	// least we want to send a cancel signal explicitly.
	cancelInput()

	inspectResp, err := s.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		slog.Error("Failed to inspect exec after completion", "err", err, "exec_id", execResp.ID)
		return status.Errorf(codes.Internal, "inspect exec: %v", err)
	}

	// Send the exit code
	slog.Debug("Sending exec exit code", "exec_id", execResp.ID, "exit_code", inspectResp.ExitCode)
	if err := stream.Send(&pb.ExecContainerResponse{
		Payload: &pb.ExecContainerResponse_ExitCode{ExitCode: int32(inspectResp.ExitCode)},
	}); err != nil {
		slog.Error("Failed to send exec exit code", "err", err, "exec_id", execResp.ID)
		return status.Errorf(codes.Internal, "send exit code: %v", err)
	}

	return nil
}
