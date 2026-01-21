package docker

// This file handles one-time migration of Caddy containers from default namespace to system namespace.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
)

// migrateLocalCaddyNamespace migrates local Caddy containers from the default namespace to the system namespace.
// Each node handles its own Caddy containers independently - no cluster coordination required.
func (s *Server) migrateLocalCaddyNamespace(ctx context.Context) error {
	containers, err := s.client.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", api.LabelServiceName+"=caddy"),
		),
	})
	if err != nil {
		return fmt.Errorf("list local caddy containers: %w", err)
	}

	for _, ctr := range containers {
		ns := ctr.Labels[api.LabelNamespace]
		// Only migrate containers that have no namespace label or are in default namespace
		if ns != "" && ns != api.DefaultNamespace {
			continue
		}

		slog.Info("Migrating local Caddy container to system namespace.",
			"container_id", ctr.ID[:12],
			"container_name", ctr.Names[0],
		)

		if err := s.recreateContainerWithNamespace(ctx, ctr.ID, api.SystemNamespace); err != nil {
			return fmt.Errorf("recreate container %s: %w", ctr.ID[:12], err)
		}

		slog.Info("Successfully migrated local Caddy container to system namespace.",
			"container_id", ctr.ID[:12],
		)
	}

	return nil
}

// recreateContainerWithNamespace stops, removes, and recreates a container with the specified namespace.
func (s *Server) recreateContainerWithNamespace(ctx context.Context, containerID string, namespace string) error {
	serviceCtr, err := s.service.InspectServiceContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("inspect container: %w", err)
	}

	wasRunning := serviceCtr.Container.State != nil && serviceCtr.Container.State.Running

	spec := serviceCtr.ServiceSpec
	spec.Namespace = namespace

	containerName := strings.TrimPrefix(serviceCtr.Container.Name, "/")

	timeout := 10
	if err := s.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("stop container: %w", err)
		}
	}

	if err := s.client.ContainerRemove(ctx, containerID, container.RemoveOptions{}); err != nil {
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("remove container: %w", err)
		}
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM containers WHERE id = $1`, containerID); err != nil {
		slog.Warn("Failed to delete old container from database.", "err", err, "id", containerID)
	}

	specBytes, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshal service spec: %w", err)
	}

	resp, err := s.CreateServiceContainer(ctx, &pb.CreateServiceContainerRequest{
		ServiceId:     serviceCtr.ServiceID(),
		ServiceSpec:   specBytes,
		ContainerName: containerName,
	})
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}

	var createResp container.CreateResponse
	if err := json.Unmarshal(resp.Response, &createResp); err != nil {
		return fmt.Errorf("unmarshal create response: %w", err)
	}

	if wasRunning {
		if err := s.client.ContainerStart(ctx, createResp.ID, container.StartOptions{}); err != nil {
			return fmt.Errorf("start container: %w", err)
		}
	}

	return nil
}
