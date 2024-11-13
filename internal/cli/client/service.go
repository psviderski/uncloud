package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
	"slices"
	"uncloud/internal/machine/api/pb"
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

	var machine *pb.MachineMember
	if opts.Machine != "" {
		// Check if the machine ID or name exists if it's explicitly specified.
		for _, m := range listResp.Machines {
			if m.Machine.Name == opts.Machine || m.Machine.Id == opts.Machine {
				machine = m
				break
			}
			return resp, fmt.Errorf("machine %q not found", opts.Machine)
		}
	} else {
		machine, err = firstAvailableMachine(listResp.Machines)
		if err != nil {
			return resp, err
		}
	}
	if machine == nil { // This should never happen.
		return resp, errors.New("no available machine to run the service")
	}

	machineIP, _ := machine.Machine.Network.ManagementIp.ToAddr()
	resp.MachineName = machine.Machine.Name

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

func firstAvailableMachine(machines []*pb.MachineMember) (*pb.MachineMember, error) {
	// Find the first UP machine.
	upIdx := slices.IndexFunc(machines, func(m *pb.MachineMember) bool {
		return m.State == pb.MachineMember_UP
	})
	if upIdx != -1 {
		return machines[upIdx], nil
	}
	// There is no UP machine, try to find the first SUSPECT machine.
	suspectIdx := slices.IndexFunc(machines, func(m *pb.MachineMember) bool {
		return m.State == pb.MachineMember_SUSPECT
	})
	if suspectIdx != -1 {
		return machines[suspectIdx], nil
	}

	return nil, errors.New("no available machine to run the service")
}
