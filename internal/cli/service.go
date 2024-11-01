package cli

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
)

// ServiceOptions contains all the options for creating a service.
type ServiceOptions struct {
	Image   string
	Name    string
	Publish []string
}

func (cli *CLI) RunService(ctx context.Context, clusterName string, opts *ServiceOptions) error {
	c, err := cli.ConnectCluster(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer func() {
		_ = c.Close()
	}()

	config := &container.Config{
		Image: opts.Image,
	}
	// TODO: generate a container name from the service name.
	// TODO: set service labels on the container.
	resp, err := c.CreateContainer(ctx, config, nil, nil, nil, opts.Name)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}

	if err = c.StartContainer(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	fmt.Printf("Service %q started with container ID %q\n", opts.Name, resp.ID)
	return nil
}
