package docker

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/jmoiron/sqlx"
	"github.com/psviderski/uncloud/pkg/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Service provides higher-level Docker operations that extends Docker API with Uncloud-specific data
// from the machine database.
type Service struct {
	// Client is a Docker client for managing Docker resources.
	Client *client.Client
	// db is a connection to the machine database.
	db *sqlx.DB
}

// NewService creates a new Docker service instance.
func NewService(client *client.Client, db *sqlx.DB) *Service {
	return &Service{
		Client: client,
		db:     db,
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

	serviceCtr.Container = api.Container{InspectResponse: ctr}

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
	// Images is a list of images present in the Docker image store (either internal or containerd).
	Images []image.Summary
	// ContainerdStore indicates whether Docker is using the containerd image store.
	ContainerdStore bool
}

// ListImages lists Docker images with the given options and indicates whether Docker is using the containerd
// image store. It always includes image manifests in the response if the store is containerd.
func (s *Service) ListImages(ctx context.Context, opts image.ListOptions) (Images, error) {
	var imagesResp Images

	// Always include the image manifests in the response.
	opts.Manifests = true
	images, err := s.Client.ImageList(ctx, opts)
	if err != nil {
		return imagesResp, status.Errorf(codes.Internal, "list images: %v", err)
	}

	isContainerdStore, err := s.IsContainerdImageStoreEnabled(ctx)
	if err != nil {
		return imagesResp, status.Errorf(codes.Internal, "check if Docker uses containerd image store: %v", err)
	}

	imagesResp = Images{
		Images:          images,
		ContainerdStore: isContainerdStore,
	}

	return imagesResp, nil
}

// ContainerLogsOptions specifies parameters for ContainerLogs.
type ContainerLogsOptions struct {
	ContainerID string
	Follow      bool
	Tail        int
	Since       string
	Until       string
}

// ContainerLogs streams logs from a container and returns demultiplexed entries via a channel.
// The channel is closed when streaming completes or context is cancelled.
func (s *Service) ContainerLogs(ctx context.Context, opts ContainerLogsOptions) (<-chan api.ContainerLogEntry, error) {
	dockerOpts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     opts.Follow,
		Tail:       strconv.FormatInt(int64(opts.Tail), 10),
		Since:      opts.Since,
		Until:      opts.Until,
		Timestamps: true,
	}

	reader, err := s.Client.ContainerLogs(ctx, opts.ContainerID, dockerOpts)
	if err != nil {
		return nil, err
	}

	outCh := make(chan api.ContainerLogEntry)
	stdoutWriter := &logsChannelWriter{ctx: ctx, ch: outCh, isStderr: false}
	stderrWriter := &logsChannelWriter{ctx: ctx, ch: outCh, isStderr: true}

	// Wrap the context in a cancellable one to unblock the second goroutine below when StdCopy completes.
	ctx, cancel := context.WithCancel(ctx)

	// Run StdCopy in a goroutine to be able to handle context cancellation.
	go func() {
		defer close(outCh)
		defer cancel()

		// StdCopy is blocking and will return when the reader is closed in another goroutine below or on error.
		if _, err := stdcopy.StdCopy(stdoutWriter, stderrWriter, reader); err != nil {
			// Send error as the last entry.
			select {
			case outCh <- api.ContainerLogEntry{Err: fmt.Errorf("demultiplex container logs: %w", err)}:
			case <-ctx.Done():
			}
		}
	}()

	// Close the reader when the context is done to cancel StdCopy if it's still running.
	go func() {
		<-ctx.Done()
		reader.Close()
	}()

	return outCh, nil
}

// logsChannelWriter is a writer for stdcopy.StdCopy that sends demultiplexed container logs to a channel.
type logsChannelWriter struct {
	ctx      context.Context
	ch       chan<- api.ContainerLogEntry
	isStderr bool
}

func (w *logsChannelWriter) Write(data []byte) (n int, err error) {
	// Parse timestamp and message from the demultiplexed Docker log payload if the data looks like it contains one.
	// Format: 2025-01-01T00:00:00.000000000Z message
	timestamp := time.Time{}
	message := data
	if len(data) > 30 && data[4] == '-' && data[7] == '-' && data[10] == 'T' {
		timestampPart, messagePart, found := bytes.Cut(data, []byte(" "))
		if found {
			timestamp, err = time.Parse(time.RFC3339Nano, string(timestampPart))
			if err != nil {
				timestamp = time.Time{}
			}
			message = messagePart
		}
	}

	entry := api.ContainerLogEntry{
		Timestamp: timestamp,
		// Clone is required because message is a slice into data, which stdcopy.StdCopy may reuse
		// after Write returns but before the entry is consumed from the channel.
		Message: bytes.Clone(message),
	}
	if w.isStderr {
		entry.Stream = api.LogStreamStderr
	} else {
		entry.Stream = api.LogStreamStdout
	}

	select {
	case w.ch <- entry:
		return len(data), nil
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	}
}
