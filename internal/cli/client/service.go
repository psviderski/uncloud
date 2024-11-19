package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/distribution/reference"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
	"slices"
	"strings"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/docker"
	"uncloud/internal/machine/docker/container"
	"uncloud/internal/secret"
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

	image, err := reference.ParseDockerRef(opts.Image)
	if err != nil {
		return resp, fmt.Errorf("invalid image: %w", err)
	}

	// Find a machine to run the service on.
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
		}
		if machine == nil {
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

	// Proxy Docker gRPC requests to the selected machine.
	machineIP, _ := machine.Machine.Network.ManagementIp.ToAddr()
	md := metadata.Pairs("machines", machineIP.String())
	ctx = metadata.NewOutgoingContext(ctx, md)

	serviceID, err := secret.NewID()
	if err != nil {
		return resp, fmt.Errorf("generate service ID: %w", err)
	}

	serviceName := opts.Name
	// Generate a random service name if not specified.
	if serviceName == "" {
		// Get the image name without the repository and tag/digest parts.
		imageName := reference.FamiliarName(image)
		// Get the last part of the image name (path), e.g. "nginx" from "bitnami/nginx".
		if i := strings.LastIndex(imageName, "/"); i != -1 {
			imageName = imageName[i+1:]
		}
		// Append a random suffix to the image name to generate an optimistically unique service name.
		suffix, err := secret.RandomAlphaNumeric(4)
		if err != nil {
			return resp, fmt.Errorf("generate random suffix: %w", err)
		}
		serviceName = fmt.Sprintf("%s-%s", imageName, suffix)
	}

	suffix, err := secret.RandomAlphaNumeric(4)
	if err != nil {
		return resp, fmt.Errorf("generate random suffix: %w", err)
	}
	containerName := fmt.Sprintf("%s-%s", serviceName, suffix)

	config := &dockercontainer.Config{
		Image: opts.Image,
		Labels: map[string]string{
			container.LabelServiceID:   serviceID,
			container.LabelServiceName: serviceName,
		},
	}
	netConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			docker.NetworkName: {},
		},
	}
	// TODO: pull image if it doesn't exist on the machine.
	createResp, err := c.CreateContainer(ctx, config, nil, netConfig, nil, containerName)
	if err != nil {
		return resp, fmt.Errorf("create container: %w", err)
	}
	if err = c.StartContainer(ctx, createResp.ID, dockercontainer.StartOptions{}); err != nil {
		return resp, fmt.Errorf("start container: %w", err)
	}

	resp.ID = serviceID
	resp.Name = serviceName
	resp.MachineName = machine.Machine.Name
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
