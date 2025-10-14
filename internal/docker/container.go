package docker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/go-connections/nat"
)

// CreateContainerWithImagePull creates a container. If the image is missing, it pulls the image first.
func (cli *Client) CreateContainerWithImagePull(
	ctx context.Context, name string, config *container.Config, hostConfig *container.HostConfig,
) (container.CreateResponse, error) {
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, name)
	if err == nil || !errdefs.IsNotFound(err) {
		return resp, err
	}

	pullCh, err := cli.PullImage(ctx, config.Image, image.PullOptions{})
	if err != nil {
		return resp, fmt.Errorf("pull image: %w", err)
	}

	// Drain the pull channel until it's closed (image is fully pulled) or an error occurs.
	// If the context is canceled during the pull, the channel will receive a context cancellation error.
	for msg := range pullCh {
		if msg.Err != nil {
			return resp, fmt.Errorf("pull image: %w", msg.Err)
		}
	}

	// Create container again after image pull.
	if resp, err = cli.ContainerCreate(ctx, config, hostConfig, nil, nil, name); err != nil {
		return resp, err
	}

	return resp, nil
}

// WaitPortPublished waits for a container port to be published on the host which happens asynchronously.
func (cli *Client) WaitPortPublished(ctx context.Context, containerID string, port nat.Port) ([]nat.PortBinding, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for {
		c, err := cli.ContainerInspect(ctx, containerID)
		if err != nil {
			return nil, fmt.Errorf("inspect container: %w", err)
		}

		binding, ok := c.NetworkSettings.Ports[port]
		if ok && len(binding) > 0 {
			return binding, nil
		}

		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, errors.New("timeout")
			}
			return nil, ctx.Err()
		}
	}
}
