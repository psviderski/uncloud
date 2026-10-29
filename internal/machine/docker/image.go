package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/pkg/stringid"
	"github.com/psviderski/uncloud/pkg/api"
)

// RemoveImage removes an image while protecting names used by containers, even when force is requested.
func (s *Service) RemoveImage(ctx context.Context, id string, opts image.RemoveOptions) ([]image.DeleteResponse, error) {
	if err := s.checkImageRemovalConflicts(ctx, id); err != nil {
		return nil, err
	}
	// Docker enforces its own conflicts, including protection of a used image's final reference.
	return s.Client.ImageRemove(ctx, id, opts)
}

// checkImageRemovalConflicts rejects an image ID used by a running or stopped container, or a name used by a
// container while it still resolves to that container's image. A digest request also checks the repository tags
// Docker would remove with it. Unused aliases are allowed.
func (s *Service) checkImageRemovalConflicts(ctx context.Context, id string) error {
	img, err := s.Client.ImageInspect(ctx, id)
	if err != nil {
		return err
	}
	// Match Docker's resolved ID before parsing names, since a short ID also parses as a repository name.
	isID := strings.HasPrefix(strings.TrimPrefix(img.ID, "sha256:"), strings.TrimPrefix(id, "sha256:"))
	var named reference.Named
	if !isID {
		named, err = reference.ParseDockerRef(id)
		if err != nil {
			return errdefs.ErrInvalidArgument.WithMessage(err.Error())
		}
	}
	_, isDigest := named.(reference.Canonical)

	containers, err := s.Client.ContainerList(ctx, container.ListOptions{
		All: true, Filters: filters.NewArgs(filters.Arg("ancestor", img.ID)),
	})
	if err != nil {
		return fmt.Errorf("list containers using image: %w", err)
	}

	for _, ctr := range containers {
		// The ancestor filter can include descendants. A retagged name must also be checked against its current ID.
		if ctr.ImageID != img.ID {
			continue
		}

		usedRef, err := reference.ParseDockerRef(ctr.Image)
		// Familiar names make "alpine:latest" and "docker.io/library/alpine:latest" identify the same tag.
		removesUsedName := err == nil && named != nil && reference.FamiliarString(usedRef) == reference.FamiliarString(named)
		if isDigest && err == nil && usedRef.Name() == named.Name() {
			// Removing a digest also removes this repository's tags pointing at the same image.
			removesUsedName = true
		}
		if isID || removesUsedName {
			ctrID, state := stringid.TruncateID(ctr.ID), "stopped"
			if serviceName := ctr.Labels[api.LabelServiceName]; serviceName != "" {
				ctrID = serviceName + "/" + ctrID
			}
			if ctr.State == container.StateRunning || ctr.State == container.StatePaused {
				state = container.StateRunning
			}
			return errdefs.ErrConflict.WithMessage(
				fmt.Sprintf("image '%s' is used by container '%s' (%s)", id, ctrID, state))
		}
	}

	return nil
}
