package client

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ServiceOptions contains all the options for creating a service.
type ServiceOptions struct {
	Image   string
	Name    string
	Machine string
	Publish []string
}

type RunServiceResponse struct {
	ID          string
	Name        string
	MachineName string
}

func (c *Client) RunService(ctx context.Context, opts *ServiceOptions) (RunServiceResponse, error) {
	var resp RunServiceResponse

	listResp, err := c.ListMachines(ctx, &emptypb.Empty{})
	if err != nil {
		return resp, fmt.Errorf("list machines: %w", err)
	}

	// TODO: find the first available machine (state UP).
	machineIP, _ := listResp.Machines[0].Machine.Network.ManagementIp.ToAddr()
	resp.MachineName = listResp.Machines[0].Machine.Name
	if opts.Machine != "" {
		for _, m := range listResp.Machines {
			if m.Machine.Name == opts.Machine || m.Machine.Id == opts.Machine {
				machineIP, _ = m.Machine.Network.ManagementIp.ToAddr()
				resp.MachineName = m.Machine.Name
				break
			}
		}
	}

	md := metadata.Pairs("machines", machineIP.String())
	ctx = metadata.NewOutgoingContext(ctx, md)

	// TODO: generate a random service ID.
	resp.ID = "todo-service-id"
	// TODO: generate a random service name if not specified.
	resp.Name = opts.Name
	// TODO: generate a container name from the service name.
	// TODO: set service labels on the container.

	config := &container.Config{
		Image: opts.Image,
	}
	createResp, err := c.CreateContainer(ctx, config, nil, nil, nil, opts.Name)
	if err != nil {
		return resp, fmt.Errorf("create container: %w", err)
	}
	if err = c.StartContainer(ctx, createResp.ID, container.StartOptions{}); err != nil {
		return resp, fmt.Errorf("start container: %w", err)
	}

	return resp, nil
}
