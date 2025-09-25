package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// CreateContainerWithImagePull creates a Docker container. If the image is missing, it pulls the image first.
func (cli *Client) CreateContainerWithImagePull(
	ctx context.Context, name string, config *container.Config, hostConfig *container.HostConfig,
) (container.CreateResponse, error) {
	var resp container.CreateResponse

	_, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, name)
	if err == nil {
		return resp, nil
	}

	if !client.IsErrNotFound(err) {
		return resp, fmt.Errorf("create container: %w", err)
	}

	pullCh, err := cli.PullImage(ctx, config.Image, image.PullOptions{})
	if err != nil {
		return resp, fmt.Errorf("pull image: %w", err)
	}

	for msg := range pullCh {
		if msg.Err != nil {
			return resp, fmt.Errorf("pull image: %w", msg.Err)
		}
	}

	// Create container again after image pull.
	if resp, err = cli.ContainerCreate(ctx, config, hostConfig, nil, nil, name); err != nil {
		return resp, fmt.Errorf("create container: %w", err)
	}

	return resp, nil
}
