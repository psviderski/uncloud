package docker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/jmoiron/sqlx"
	"github.com/psviderski/uncloud/internal/containerd"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Service provides higher-level Docker operations that extends Docker API with Uncloud-specific data
// from the machine database.
type Service struct {
	// Client is a Docker client for managing Docker resources.
	Client *client.Client
	// containerd is a containerd client for accessing containerd images.
	containerd *containerd.Client
	// db is a connection to the machine database.
	db *sqlx.DB
}

// NewService creates a new Docker service instance.
func NewService(client *client.Client, containerdClient *containerd.Client, db *sqlx.DB) *Service {
	return &Service{
		Client:     client,
		containerd: containerdClient,
		db:         db,
	}
}

// InspectServiceContainer inspects a Docker container and retrieves its associated ServiceSpec
// from the machine database, returning a complete ServiceContainer.
func (s *Service) InspectServiceContainer(ctx context.Context, nameOrID string) (api.ServiceContainer, error) {
	var serviceCtr api.ServiceContainer

	ctr, err := s.Client.ContainerInspect(ctx, nameOrID)
	if err != nil {
		return serviceCtr, err
	}
	if _, ok := ctr.Config.Labels[api.LabelManaged]; !ok {
		return serviceCtr, fmt.Errorf("container '%s' is not managed by Uncloud", nameOrID)
	}

	serviceCtr.Container = api.Container{ContainerJSON: ctr}

	// Retrieve ServiceSpec from the machine database.
	var specBytes []byte
	err = s.db.QueryRowContext(ctx, `SELECT service_spec FROM containers WHERE id = $1`, ctr.ID).Scan(&specBytes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// If this happens, there is a bug in the code, or someone manually removed the container from the DB,
			// or created a managed container out of band or by previous uncloud installation.
			return serviceCtr, fmt.Errorf("service spec not found for container '%s' in machine DB", ctr.ID)
		}
		return serviceCtr, fmt.Errorf("get service spec for container '%s' from machine DB: %w", ctr.ID, err)
	}

	if err = json.Unmarshal(specBytes, &serviceCtr.ServiceSpec); err != nil {
		return serviceCtr, fmt.Errorf("unmarshal service spec for container '%s': %w", ctr.ID, err)
	}

	return serviceCtr, nil
}

// ListServiceContainers lists Docker containers that belong to the service with the given name or ID.
// If serviceIDOrName is empty, all service containers are returned. The opts parameter allows additional filtering.
func (s *Service) ListServiceContainers(
	ctx context.Context, serviceNameOrID string, opts container.ListOptions,
) ([]api.ServiceContainer, error) {
	if opts.Filters.Len() == 0 {
		opts.Filters = filters.NewArgs()
	}
	// Add labels to existing filters to list only Uncloud-managed service containers.
	opts.Filters.Add("label", api.LabelServiceID)
	opts.Filters.Add("label", api.LabelManaged)

	containerSummaries, err := s.Client.ContainerList(ctx, opts)
	if err != nil {
		return nil, err
	}

	var containers []api.ServiceContainer
	for _, cs := range containerSummaries {
		// Filter by service name or ID if provided.
		if serviceNameOrID != "" &&
			cs.Labels[api.LabelServiceID] != serviceNameOrID &&
			cs.Labels[api.LabelServiceName] != serviceNameOrID {
			continue
		}

		ctr, err := s.InspectServiceContainer(ctx, cs.ID)
		if err != nil {
			// Log error but continue with other containers.
			slog.Error("Failed to inspect service container.", "service", serviceNameOrID, "id", cs.ID, "err", err)
			continue
		}
		containers = append(containers, ctr)
	}

	return containers, nil
}

// IsContainerdImageStoreEnabled checks if Docker is configured to use the containerd image store:
// https://docs.docker.com/engine/storage/containerd/
func (s *Service) IsContainerdImageStoreEnabled(ctx context.Context) (bool, error) {
	info, err := s.Client.Info(ctx)
	if err != nil {
		return false, fmt.Errorf("get Docker info: %w", err)
	}

	return strings.Contains(fmt.Sprintf("%s", info.DriverStatus), "containerd.snapshotter"), nil
}

type Images struct {
	// DockerImages lists images from the Docker internal image store.
	// It may be empty if Docker uses the containerd image store.
	DockerImages []image.Summary
	// ContainerdImages lists images from the containerd image store.
	ContainerdImages []image.Summary
	// DockerContainerdStore indicates whether Docker is using the containerd image store.
	DockerContainerdStore bool
}

// ListImages lists images present in the Docker and containerd image stores.
// If Docker uses the containerd image store, only images from containerd are listed, as the internal Docker image
// store is not accessible in this case. Otherwise, images from both stores are listed.
func (s *Service) ListImages(ctx context.Context, opts image.ListOptions) (Images, error) {
	var images Images

	// Always include the image manifests in the response.
	opts.Manifests = true
	// List images from Docker.
	dockerImages, err := s.Client.ImageList(ctx, opts)
	if err != nil {
		return images, status.Errorf(codes.Internal, "list Docker images: %v", err)
	}

	isContainerdStore, err := s.IsContainerdImageStoreEnabled(ctx)
	if err != nil {
		return images, status.Errorf(codes.Internal, "check if Docker uses containerd image store: %v", err)
	}

	if isContainerdStore {
		// Docker uses the containerd image store, hence the images listed from Docker are just references to images
		// in containerd. The internal Docker image store is not accessible in this case.
		images = Images{
			ContainerdImages:      dockerImages,
			DockerContainerdStore: true,
		}
	} else {
		images = Images{
			DockerImages: dockerImages,
			// TODO: List images from containerd directly. ContainerdImages: containerdImages,
		}
	}

	return images, nil
}
