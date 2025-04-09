package docker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/distribution/reference"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/jmoiron/sqlx"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var fullDockerIDRegex = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Server implements the gRPC Docker service that proxies requests to the Docker daemon.
type Server struct {
	pb.UnimplementedDockerServer
	client *client.Client
	db     *sqlx.DB
}

// NewServer creates a new Docker gRPC server with the provided Docker client.
func NewServer(cli *client.Client, db *sqlx.DB) *Server {
	return &Server{
		client: cli,
		db:     db,
	}
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
			return nil, status.Errorf(codes.NotFound, err.Error())
		}
		return nil, status.Errorf(codes.Internal, err.Error())
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
		if client.IsErrNotFound(err) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		}
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal response: %v", err)
	}

	return &pb.InspectContainerResponse{Response: respBytes}, nil
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
		if client.IsErrNotFound(err) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		}
		return nil, status.Errorf(codes.Internal, err.Error())
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
		if client.IsErrNotFound(err) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		}
		return nil, status.Errorf(codes.Internal, err.Error())
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
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	containers := make([]types.ContainerJSON, 0, len(containerSummaries))
	for _, cs := range containerSummaries {
		c, err := s.client.ContainerInspect(ctx, cs.ID)
		if err != nil {
			if client.IsErrNotFound(err) {
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
		if client.IsErrNotFound(err) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		}
		return nil, status.Errorf(codes.Internal, err.Error())
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
		return status.Errorf(codes.Internal, err.Error())
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
			return status.Errorf(codes.Canceled, ctx.Err().Error())
		}
	}
}

// InspectImage returns the image information for the given image ID.
func (s *Server) InspectImage(ctx context.Context, req *pb.InspectImageRequest) (*pb.InspectImageResponse, error) {
	resp, _, err := s.client.ImageInspectWithRaw(ctx, req.Id)
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		}
		return nil, status.Errorf(codes.Internal, err.Error())
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

// CreateVolume creates a new volume with the given options.
func (s *Server) CreateVolume(ctx context.Context, req *pb.CreateVolumeRequest) (*pb.CreateVolumeResponse, error) {
	var opts volume.CreateOptions
	if err := json.Unmarshal(req.Options, &opts); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unmarshal options: %v", err)
	}

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
		if client.IsErrNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &emptypb.Empty{}, nil
}

// CreateServiceContainer creates a new container for the service with the given specifications.
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

	config := &container.Config{
		Cmd:        spec.Container.Command,
		Env:        spec.Container.Env.ToSlice(),
		Entrypoint: spec.Container.Entrypoint,
		Hostname:   containerName,
		Image:      spec.Container.Image,
		Labels: map[string]string{
			api.LabelServiceID:   req.ServiceId,
			api.LabelServiceName: spec.Name,
			api.LabelServiceMode: spec.Mode,
			api.LabelManaged:     "",
		},
	}
	if spec.Mode == "" {
		config.Labels[api.LabelServiceMode] = api.ServiceModeReplicated
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

	mounts, err := toDockerMounts(spec.Volumes, spec.Container.VolumeMounts)
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
		Binds:        spec.Container.Volumes,
		Init:         spec.Container.Init,
		Mounts:       mounts,
		PortBindings: portBindings,
		// Always restart service containers if they exit or a machine restarts.
		// For one-off containers and batch jobs we plan to use a different service type/mode.
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyAlways,
		},
	}
	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			NetworkName: {},
		},
	}

	resp, err := s.client.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, containerName)
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
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

	if _, err = s.db.ExecContext(ctx,
		`INSERT INTO containers (id, service_id, service_spec) VALUES ($1, $2, $3)`,
		resp.ID, req.ServiceId, string(specBytes)); err != nil {
		removeContainer()
		return nil, status.Errorf(codes.Internal, "store container in database: %v", err)
	}

	return &pb.CreateContainerResponse{Response: respBytes}, nil
}

func toDockerMounts(volumes []api.VolumeSpec, mounts []api.VolumeMount) ([]mount.Mount, error) {
	dockerMounts := make([]mount.Mount, 0, len(mounts))
	for _, m := range mounts {
		idx := slices.IndexFunc(volumes, func(v api.VolumeSpec) bool {
			return v.Name == m.VolumeName
		})
		if idx == -1 {
			return nil, fmt.Errorf("volume mount references a volume that doesn't exist in the volumes spec: '%s'",
				m.VolumeName)
		}

		vol := volumes[idx]
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
			dm.Source = vol.Name

			if vol.VolumeOptions != nil {
				dm.VolumeOptions = &mount.VolumeOptions{
					NoCopy:       vol.VolumeOptions.NoCopy,
					Labels:       vol.VolumeOptions.Labels,
					Subpath:      vol.VolumeOptions.SubPath,
					DriverConfig: vol.VolumeOptions.Driver,
				}

				if vol.VolumeOptions.Name != "" {
					dm.Source = vol.VolumeOptions.Name
				}
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

// verifyDockerVolumesExist checks if the Docker named volumes referenced in the mounts exist on the machine.
func (s *Server) verifyDockerVolumesExist(ctx context.Context, mounts []mount.Mount) error {
	for _, m := range mounts {
		if m.Type != mount.TypeVolume {
			continue
		}

		// TODO: non-local volume drivers should likely be handled differently (needs proper investigation).
		if _, err := s.client.VolumeInspect(ctx, m.Source); err != nil {
			if client.IsErrNotFound(err) {
				return status.Errorf(codes.NotFound, "volume '%s' not found", m.Source)
			}
			return status.Errorf(codes.Internal, "inspect volume '%s': %v", m.Source, err.Error())
		}
		// TODO: check if the volume driver and options are the same as in the mount and fail if not.
	}

	return nil
}

// InspectServiceContainer returns the container information and service specification that was used to create the
// container with the given ID.
func (s *Server) InspectServiceContainer(
	ctx context.Context, req *pb.InspectContainerRequest,
) (*pb.ServiceContainer, error) {
	ctr, err := s.client.ContainerInspect(ctx, req.Id)
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		}
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	ctrBytes, err := json.Marshal(ctr)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal response: %v", err)
	}

	var specBytes []byte
	err = s.db.QueryRowContext(ctx, `SELECT service_spec FROM containers WHERE id = $1`, ctr.ID).Scan(&specBytes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "service spec not found for container: '%s'", ctr.ID)
		}
		return nil, status.Errorf(codes.Internal, "get service spec for container '%s' from machine database: %v",
			ctr.ID, err)
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
		} else {
			opts.Filters = filters.NewArgs()
		}
	}
	// Only uncloud-managed containers that belong to some service.
	opts.Filters.Add("label", api.LabelServiceID)
	opts.Filters.Add("label", api.LabelManaged)

	containerSummaries, err := s.client.ContainerList(ctx, opts)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	containers := make([]*pb.ServiceContainer, 0, len(containerSummaries))
	for _, cs := range containerSummaries {
		if req.ServiceId != "" &&
			cs.Labels[api.LabelServiceID] != req.ServiceId && cs.Labels[api.LabelServiceName] != req.ServiceId {
			continue
		}

		ctr, err := s.client.ContainerInspect(ctx, cs.ID)
		if err != nil {
			if client.IsErrNotFound(err) {
				// The listed container may have been removed while we were inspecting other containers.
				continue
			}
			return nil, status.Errorf(codes.Internal, "inspect container %s: %v", cs.ID, err)
		}
		ctrBytes, err := json.Marshal(ctr)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "marshal container: %v", err)
		}

		var specBytes []byte
		err = s.db.QueryRowContext(ctx, `SELECT service_spec FROM containers WHERE id = $1`, ctr.ID).Scan(&specBytes)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// If this happens, there is a bug in the code, or someone manually removed the container from the DB,
				// or created a managed container out of band.
				slog.Error("Service container not found in machine database.", "id", ctr.ID)
				// Just ignore such a container to not fail the list operation as it's not easily recoverable.
				continue
			}
			return nil, status.Errorf(codes.Internal, "get service spec for container '%s' from machine database: %v",
				ctr.ID, err)
		}

		containers = append(containers, &pb.ServiceContainer{
			Container:   ctrBytes,
			ServiceSpec: specBytes,
		})
	}

	return &pb.ListServiceContainersResponse{
		Messages: []*pb.MachineServiceContainers{
			{
				Containers: containers,
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
			if client.IsErrNotFound(err) {
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
