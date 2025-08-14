package docker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/jmoiron/sqlx"
	"github.com/psviderski/uncloud/pkg/api"
)

// Service provides higher-level Docker operations that extends Docker API with Uncloud-specific data
// from the machine database.
type Service struct {
	Client *client.Client
	db     *sqlx.DB
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
