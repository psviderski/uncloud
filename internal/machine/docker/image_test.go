package docker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCheckImageRemovalConflicts(t *testing.T) {
	imageID := "sha256:" + strings.Repeat("a", 64)
	otherID := "sha256:" + strings.Repeat("b", 64)
	digestRef := "myapp@" + imageID

	tests := []struct {
		name             string
		requested        string
		configured       string
		containerImageID string
		serviceName      string
		blocked          bool
	}{
		{name: "used tag", requested: "myapp:current", configured: "myapp:current", serviceName: "web", blocked: true},
		{name: "qualified request", requested: "docker.io/library/myapp:current", configured: "myapp:current", blocked: true},
		{name: "qualified container reference", requested: "myapp:current", configured: "docker.io/library/myapp:current", blocked: true},
		{name: "implicit latest", requested: "myapp", configured: "myapp:latest", blocked: true},
		{name: "unused alias", requested: "myapp:old", configured: "myapp:current"},
		{name: "unused alias of ID-used image", requested: "myapp:old", configured: imageID},
		{name: "tag may clean up a used digest", requested: "myapp:old", configured: digestRef},
		{name: "used digest", requested: digestRef, configured: digestRef, blocked: true},
		{name: "digest also removes used tag", requested: digestRef, configured: "myapp:current", blocked: true},
		{name: "unused repository digest", requested: digestRef, configured: "other:keep"},
		{name: "image ID", requested: imageID, configured: "myapp:current", blocked: true},
		{name: "short image ID", requested: "aaaaaaaa", configured: "myapp:current", blocked: true},
		{name: "short image ID with algorithm", requested: "sha256:aaaaaaaa", configured: "myapp:current", blocked: true},
		{name: "retagged name", requested: "myapp:current", configured: "myapp:current", containerImageID: otherID},
	}

	for _, tt := range tests {
		for _, state := range []string{container.StateExited, container.StateRunning} {
			t.Run(fmt.Sprintf("%s/%s", tt.name, state), func(t *testing.T) {
				containerImageID := tt.containerImageID
				if containerImageID == "" {
					containerImageID = imageID
				}

				cli := NewMockAPIClient(t)
				cli.EXPECT().ImageInspect(mock.Anything, tt.requested).
					Return(image.InspectResponse{ID: imageID}, nil).Once()
				cli.EXPECT().ContainerList(mock.Anything, mock.MatchedBy(func(opts container.ListOptions) bool {
					return opts.All // Stopped containers must be checked too.
				})).Return([]container.Summary{{
					ID:      strings.Repeat("c", 64),
					Labels:  map[string]string{api.LabelServiceName: tt.serviceName},
					Names:   []string{"/web"},
					ImageID: containerImageID,
					Image:   tt.configured,
					State:   state,
				}}, nil).Once()

				service := NewService(cli, nil)
				err := service.checkImageRemovalConflicts(context.Background(), tt.requested)

				if tt.blocked {
					require.True(t, errdefs.IsConflict(err), "%v", err)
					assert.ErrorContains(t, err, tt.requested)
					if tt.serviceName != "" {
						assert.ErrorContains(t, err, "container 'web/cccccccccccc'")
					} else {
						assert.ErrorContains(t, err, "container 'cccccccccccc'")
					}
					wantState := "stopped"
					if state == container.StateRunning {
						wantState = state
					}
					assert.ErrorContains(t, err, wantState)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}

func TestCheckImageRemovalConflicts_PropagatesLookupErrors(t *testing.T) {
	imageID := "sha256:" + strings.Repeat("a", 64)
	t.Run("image lookup fails", func(t *testing.T) {
		cli := NewMockAPIClient(t)
		lookupErr := errdefs.ErrNotFound.WithMessage("no such image")
		cli.EXPECT().ImageInspect(mock.Anything, "myapp:old").Return(image.InspectResponse{}, lookupErr).Once()

		service := NewService(cli, nil)
		require.ErrorIs(t, service.checkImageRemovalConflicts(context.Background(), "myapp:old"), lookupErr)
	})

	t.Run("container lookup fails", func(t *testing.T) {
		cli := NewMockAPIClient(t)
		lookupErr := errors.New("Docker unavailable")
		cli.EXPECT().ImageInspect(mock.Anything, "myapp:old").Return(image.InspectResponse{ID: imageID}, nil).Once()
		cli.EXPECT().ContainerList(mock.Anything, mock.Anything).Return(nil, lookupErr).Once()

		service := NewService(cli, nil)
		err := service.checkImageRemovalConflicts(context.Background(), "myapp:old")

		require.ErrorIs(t, err, lookupErr)
		assert.ErrorContains(t, err, "list containers using image")
	})
}

func TestServiceRemoveImage_RejectsUsedNameBeforeDeletion(t *testing.T) {
	imageID := "sha256:" + strings.Repeat("a", 64)
	cli := NewMockAPIClient(t)
	cli.EXPECT().ImageInspect(mock.Anything, "myapp:current").Return(image.InspectResponse{ID: imageID}, nil).Once()
	cli.EXPECT().ContainerList(mock.Anything, mock.Anything).Return([]container.Summary{{
		ID: "container-id", ImageID: imageID, Image: "myapp:current", State: container.StateRunning,
	}}, nil).Once()

	service := NewService(cli, nil)
	// ImageRemove has no expectation: any deletion call would fail this test.
	results, err := service.RemoveImage(context.Background(), "myapp:current", image.RemoveOptions{Force: true})

	require.True(t, errdefs.IsConflict(err), "%v", err)
	assert.Nil(t, results)
}

func TestServiceRemoveImage_DelegatesToDocker(t *testing.T) {
	imageID := "sha256:" + strings.Repeat("a", 64)
	tests := []struct {
		name    string
		opts    image.RemoveOptions
		results []image.DeleteResponse
		err     error
	}{
		{
			name:    "options and results pass through",
			opts:    image.RemoveOptions{Force: true, PruneChildren: true, Platforms: []ocispec.Platform{{OS: "linux", Architecture: "amd64"}}},
			results: []image.DeleteResponse{{Deleted: imageID}},
		},
		{
			name: "Docker rejects final reference removal",
			opts: image.RemoveOptions{PruneChildren: true},
			err:  errdefs.ErrConflict.WithMessage("container is using its referenced image"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := NewMockAPIClient(t)
			cli.EXPECT().ImageInspect(mock.Anything, "myapp:old").Return(image.InspectResponse{ID: imageID}, nil).Once()
			cli.EXPECT().ContainerList(mock.Anything, mock.Anything).Return([]container.Summary{{
				ID: "container-id", ImageID: imageID, Image: "myapp@" + imageID, State: container.StateRunning,
			}}, nil).Once()
			cli.EXPECT().ImageRemove(mock.Anything, "myapp:old", tt.opts).Return(tt.results, tt.err).Once()

			service := NewService(cli, nil)
			results, err := service.RemoveImage(context.Background(), "myapp:old", tt.opts)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.results, results)
		})
	}
}
