package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"slices"
	"strings"
	"sync"
	"uncloud/internal/api"
	"uncloud/internal/machine/api/pb"
	machinedocker "uncloud/internal/machine/docker"
	"uncloud/internal/secret"
	"uncloud/internal/service"
)

// ServiceOptions contains all the options for creating a service.
// TODO: replace with ServiceSpec.
type ServiceOptions struct {
	Image   string
	Name    string
	Machine string
	// Mode is the replication mode of the service.
	Mode    string
	Publish []string
}

type RunServiceResponse struct {
	ID         string
	Name       string
	Containers []api.MachineContainerID
}

func (cli *Client) RunService(ctx context.Context, spec api.ServiceSpec) (RunServiceResponse, error) {
	var resp RunServiceResponse

	img, err := reference.ParseDockerRef(spec.Container.Image)
	if err != nil {
		return resp, fmt.Errorf("invalid image: %w", err)
	}

	if spec.Name == "" {
		// Generate a random service name from the image if not specified.
		// Get the image name without the repository and tag/digest parts.
		imageName := reference.FamiliarName(img)
		// Get the last part of the image name (path), e.g. "nginx" from "bitnami/nginx".
		if i := strings.LastIndex(imageName, "/"); i != -1 {
			imageName = imageName[i+1:]
		}
		// Append a random suffix to the image name to generate an optimistically unique service name.
		suffix, err := secret.RandomAlphaNumeric(4)
		if err != nil {
			return resp, fmt.Errorf("generate random suffix: %w", err)
		}
		spec.Name = fmt.Sprintf("%s-%s", imageName, suffix)
	} else {
		// Optimistically check if a service with the specified name already exists.
		_, err := cli.InspectService(ctx, spec.Name)
		if err == nil {
			return resp, fmt.Errorf("service with name '%s' already exists", spec.Name)
		}
		if !errors.Is(err, ErrNotFound) {
			return resp, fmt.Errorf("inspect service: %w", err)
		}
	}

	serviceID, err := secret.NewID()
	if err != nil {
		return resp, fmt.Errorf("generate service ID: %w", err)
	}

	switch spec.Mode {
	case "", api.ServiceModeReplicated:
		return cli.runReplicatedService(ctx, serviceID, spec)
	case api.ServiceModeGlobal:
		return cli.runGlobalService(ctx, serviceID, spec)
	default:
		return resp, fmt.Errorf("invalid mode: %q", spec.Mode)
	}
}

func (cli *Client) runGlobalService(ctx context.Context, id string, spec api.ServiceSpec) (RunServiceResponse, error) {
	resp := RunServiceResponse{
		ID:   id,
		Name: spec.Name,
	}

	machines, err := cli.ListMachines(ctx)
	if err != nil {
		return resp, fmt.Errorf("list machines: %w", err)
	}

	for _, m := range machines {
		// Run a service container on each available machine.
		if m.State == pb.MachineMember_UP || m.State == pb.MachineMember_SUSPECT {
			// TODO: run each machine in a goroutine.
			createResp, err := cli.runContainer(ctx, id, spec, m.Machine)
			// TODO: collect errors and return after trying all machines.
			if err != nil {
				return resp, fmt.Errorf("run container: %w", err)
			}

			resp.Containers = append(resp.Containers, api.MachineContainerID{
				MachineID:   m.Machine.Id,
				ContainerID: createResp.ID,
			})
		}
	}

	return resp, nil
}

func (cli *Client) runReplicatedService(ctx context.Context, id string, spec api.ServiceSpec) (RunServiceResponse, error) {
	return RunServiceResponse{}, fmt.Errorf("replicated mode is not supported yet")
	//
	//// Find a machine to run the service on.
	//machines, err := cli.ListMachines(ctx)
	//if err != nil {
	//	return resp, fmt.Errorf("list machines: %w", err)
	//}
	//
	//var machine *pb.MachineMember
	//if opts.Machine != "" {
	//	// Check if the machine ID or name exists if it's explicitly specified.
	//	for _, m := range machines {
	//		if m.Machine.Name == opts.Machine || m.Machine.Id == opts.Machine {
	//			machine = m
	//			break
	//		}
	//	}
	//	if machine == nil {
	//		return resp, fmt.Errorf("machine %q not found", opts.Machine)
	//	}
	//} else {
	//	machine, err = firstAvailableMachine(machines)
	//	if err != nil {
	//		return resp, err
	//	}
	//}
	//if machine == nil { // This should never happen.
	//	return resp, errors.New("no available machine to run the service")
	//}
	//
	//// Proxy Docker gRPC requests to the selected machine.
	//machineIP, _ := machine.Machine.Network.ManagementIp.ToAddr()
	//md := metadata.Pairs("machines", machineIP.String())
	//ctx = metadata.NewOutgoingContext(ctx, md)
	//
	//serviceID, err := secret.NewID()
	//if err != nil {
	//	return resp, fmt.Errorf("generate service ID: %w", err)
	//}
	//
	//serviceName := opts.Name
	//// Generate a random service name if not specified.
	//if serviceName == "" {
	//	// Get the image name without the repository and tag/digest parts.
	//	imageName := reference.FamiliarName(image)
	//	// Get the last part of the image name (path), e.g. "nginx" from "bitnami/nginx".
	//	if i := strings.LastIndex(imageName, "/"); i != -1 {
	//		imageName = imageName[i+1:]
	//	}
	//	// Append a random suffix to the image name to generate an optimistically unique service name.
	//	suffix, err := secret.RandomAlphaNumeric(4)
	//	if err != nil {
	//		return resp, fmt.Errorf("generate random suffix: %w", err)
	//	}
	//	serviceName = fmt.Sprintf("%s-%s", imageName, suffix)
	//}
	//
	//suffix, err := secret.RandomAlphaNumeric(4)
	//if err != nil {
	//	return resp, fmt.Errorf("generate random suffix: %w", err)
	//}
	//containerName := fmt.Sprintf("%s-%s", serviceName, suffix)
	//
	//config := &container.Config{
	//	Image: opts.Image,
	//	Labels: map[string]string{
	//		service.LabelServiceID:   serviceID,
	//		service.LabelServiceName: serviceName,
	//	},
	//}
	//netConfig := &network.NetworkingConfig{
	//	EndpointsConfig: map[string]*network.EndpointSettings{
	//		machinedocker.NetworkName: {},
	//	},
	//}
	//// TODO: pull image if it doesn't exist on the machine.
	//createResp, err := cli.CreateContainer(ctx, config, nil, netConfig, nil, containerName)
	//if err != nil {
	//	return resp, fmt.Errorf("create container: %w", err)
	//}
	//if err = cli.StartContainer(ctx, createResp.ID, container.StartOptions{}); err != nil {
	//	return resp, fmt.Errorf("start container: %w", err)
	//}
	//
	//resp.ID = serviceID
	//resp.Name = serviceName
	//resp.MachineName = machine.Machine.Name
	//return resp, nil
}

func (cli *Client) runContainer(
	ctx context.Context, serviceID string, spec api.ServiceSpec, machine *pb.MachineInfo,
) (container.CreateResponse, error) {
	var resp container.CreateResponse

	// Proxy Docker gRPC requests to the selected machine.
	machineIP, _ := machine.Network.ManagementIp.ToAddr()
	md := metadata.Pairs("machines", machineIP.String())
	ctx = metadata.NewOutgoingContext(ctx, md)

	suffix, err := secret.RandomAlphaNumeric(4)
	if err != nil {
		return resp, fmt.Errorf("generate random suffix: %w", err)
	}
	containerName := fmt.Sprintf("%s-%s", spec.Name, suffix)

	config := &container.Config{
		Cmd:   spec.Container.Command,
		Image: spec.Container.Image,
		Labels: map[string]string{
			service.LabelServiceID:   serviceID,
			service.LabelServiceName: spec.Name,
			service.LabelManaged:     "",
		},
	}
	if spec.Mode == api.ServiceModeGlobal {
		config.Labels[service.LabelServiceMode] = api.ServiceModeGlobal
	}

	hostConfig := &container.HostConfig{
		Init: spec.Container.Init,
	}
	netConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			machinedocker.NetworkName: {},
		},
	}

	resp, err = cli.CreateContainer(ctx, config, hostConfig, netConfig, nil, containerName)
	if err != nil {
		if !dockerclient.IsErrNotFound(err) {
			return resp, fmt.Errorf("create container: %w", err)
		}

		// Pull the missing image and create the container again.
		pullCh, err := cli.PullImage(ctx, config.Image)
		if err != nil {
			return resp, fmt.Errorf("pull image: %w", err)
		}

		// Wait for pull to complete by reading all progress messages.
		for msg := range pullCh {
			// TODO: report progress.
			if msg.Err != nil {
				return resp, fmt.Errorf("pull image: %w", msg.Err)
			}
		}

		if resp, err = cli.CreateContainer(ctx, config, hostConfig, netConfig, nil, containerName); err != nil {
			return resp, fmt.Errorf("create container: %w", err)
		}
	}

	if err = cli.StartContainer(ctx, resp.ID, container.StartOptions{}); err != nil {
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

// InspectService returns detailed information about a service and its containers.
// The id parameter can be either a service ID or name.
func (cli *Client) InspectService(ctx context.Context, id string) (service.Service, error) {
	var svc service.Service

	machines, err := cli.ListMachines(ctx)
	if err != nil {
		return svc, fmt.Errorf("list machines: %w", err)
	}

	// Broadcast the container list request to all available machines.
	machineIDByManagementIP := make(map[string]string)
	md := metadata.New(nil)
	for _, m := range machines {
		if m.State == pb.MachineMember_UP || m.State == pb.MachineMember_SUSPECT {
			machineIP, _ := m.Machine.Network.ManagementIp.ToAddr()
			md.Append("machines", machineIP.String())

			machineIDByManagementIP[machineIP.String()] = m.Machine.Id
		}
		// TODO: warning about machines that are DOWN.
	}
	listCtx := metadata.NewOutgoingContext(ctx, md)

	// List only uncloud-managed containers that belong to some service.
	opts := container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", service.LabelServiceID),
			filters.Arg("label", service.LabelManaged),
		),
	}
	machineContainers, err := cli.ListContainers(listCtx, opts)
	if err != nil {
		return svc, fmt.Errorf("list containers: %w", err)
	}

	// Collect all containers on all machines that belong to the specified service.
	foundByID := false
	var containers []service.MachineContainer
	for _, mc := range machineContainers {
		// Metadata can be nil if the request was broadcasted to only one machine.
		if mc.Metadata == nil && len(machineContainers) > 1 {
			return svc, errors.New("something went wrong with gRPC proxy: metadata is missing for a machine response")
		}
		if mc.Metadata != nil && mc.Metadata.Error != "" {
			// TODO: return failed machines in the response.
			fmt.Printf("WARNING: failed to list containers on machine '%s': %s\n",
				mc.Metadata.Machine, mc.Metadata.Error)
			continue
		}

		machineID := ""
		if mc.Metadata == nil {
			// ListContainers was proxied to only one machine.
			for _, v := range machineIDByManagementIP {
				machineID = v
				break
			}
		} else {
			var ok bool
			machineID, ok = machineIDByManagementIP[mc.Metadata.Machine]
			if !ok {
				return svc, fmt.Errorf("machine name not found for management IP: %s", mc.Metadata.Machine)
			}
		}

		for _, c := range mc.Containers {
			ctr := service.Container{Container: c}
			if ctr.ServiceID() == id || ctr.ServiceName() == id {
				containers = append(containers, service.MachineContainer{
					MachineID: machineID,
					Container: ctr,
				})

				if ctr.ServiceID() == id {
					foundByID = true
				}
			}
		}
	}

	if len(containers) == 0 {
		return svc, ErrNotFound
	}

	// Containers from different services may share the same service name (distributed and eventually consistent store
	// may not prevent this), or a service name might match another service's ID. In these cases, matching by ID takes
	// priority over matching by name.
	if foundByID {
		containers = slices.DeleteFunc(containers, func(mc service.MachineContainer) bool {
			return mc.Container.ServiceID() != id
		})
	} else {
		// Matched only by name but there could be multiple services with the same name.
		serviceID := containers[0].Container.ServiceID()
		for _, mc := range containers[1:] {
			if mc.Container.ServiceID() != serviceID {
				return svc, fmt.Errorf("multiple services found with name: %s", id)
			}
		}
	}

	svc = service.Service{
		ID:         containers[0].Container.ServiceID(),
		Name:       containers[0].Container.ServiceName(),
		Mode:       containers[0].Container.ServiceMode(),
		Containers: containers,
	}
	if svc.Mode == "" {
		svc.Mode = api.ServiceModeReplicated
	}

	return svc, nil
}

// InspectServiceFromStore returns detailed information about a service and its containers from the distributed store.
// Due to eventual consistency of the store, the returned information may not reflect the most recent changes.
// The id parameter can be either a service ID or name.
func (cli *Client) InspectServiceFromStore(ctx context.Context, id string) (service.Service, error) {
	var svc service.Service

	resp, err := cli.MachineClient.InspectService(ctx, &pb.InspectServiceRequest{Id: id})
	if err != nil {
		if s, ok := status.FromError(err); ok {
			if s.Code() == codes.NotFound {
				return svc, ErrNotFound
			}
		}
		return svc, err
	}

	svc, err = service.FromProto(resp.Service)
	if err != nil {
		return svc, fmt.Errorf("from proto: %w", err)
	}
	return svc, nil
}

// RemoveService removes all containers on all machines that belong to the specified service.
// The id parameter can be either a service ID or name.
func (cli *Client) RemoveService(ctx context.Context, id string) error {
	svc, err := cli.InspectService(ctx, id)
	if err != nil {
		return err
	}

	machines, err := cli.ListMachines(ctx)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}
	machineManagementIPByID := make(map[string]string)
	for _, m := range machines {
		machineIP, _ := m.Machine.Network.ManagementIp.ToAddr()
		machineManagementIPByID[m.Machine.Id] = machineIP.String()
	}

	wg := sync.WaitGroup{}
	errCh := make(chan error)

	// Remove all containers on all machines that belong to the service.
	for _, mc := range svc.Containers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			machineIP, ok := machineManagementIPByID[mc.MachineID]
			if !ok {
				errCh <- fmt.Errorf("machine not found by ID: %s", mc.MachineID)
				return
			}
			removeCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("machines", machineIP))
			// TODO: gracefully stop the container before removing it without force.
			err := cli.RemoveContainer(removeCtx, mc.Container.ID, container.RemoveOptions{Force: true})
			if err != nil {
				if !dockerclient.IsErrNotFound(err) {
					errCh <- fmt.Errorf("remove container '%s': %w", mc.Container.ID, err)
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(errCh)
	}()

	err = nil
	for e := range errCh {
		err = errors.Join(err, e)
	}
	return err
}
