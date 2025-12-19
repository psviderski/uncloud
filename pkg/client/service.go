package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy/scheduler"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func (cli *Client) RunService(ctx context.Context, spec api.ServiceSpec) (api.RunServiceResponse, error) {
	var resp api.RunServiceResponse

	if err := spec.Validate(); err != nil {
		return resp, fmt.Errorf("invalid service spec: %w", err)
	}

	if spec.Name != "" {
		// Optimistically check if a service with the specified name already exists.
		_, err := cli.InspectService(ctx, spec.Name)
		if err == nil {
			return resp, fmt.Errorf("service with name '%s' already exists", spec.Name)
		}
		if !errors.Is(err, api.ErrNotFound) {
			return resp, fmt.Errorf("inspect service: %w", err)
		}
	}

	// Create missing named Docker volumes for the service.
	if len(spec.MountedDockerVolumes()) > 0 {
		state, err := scheduler.InspectClusterState(ctx, cli)
		if err != nil {
			return resp, fmt.Errorf("inspect cluster state: %w", err)
		}
		volumeScheduler, err := scheduler.NewVolumeScheduler(state, []api.ServiceSpec{spec})
		if err != nil {
			return resp, fmt.Errorf("init volume scheduler: %w", err)
		}

		scheduledVolumes, err := volumeScheduler.Schedule()
		if err != nil {
			return resp, fmt.Errorf("schedule volumes: %w", err)
		}

		// Create the missing volumes on the scheduled machines.
		for machineID, volumes := range scheduledVolumes {
			for _, v := range volumes {
				opts := volume.CreateOptions{
					Name: v.Name,
				}
				if v.VolumeOptions != nil {
					if v.VolumeOptions.Driver != nil {
						opts.Driver = v.VolumeOptions.Driver.Name
						opts.DriverOpts = v.VolumeOptions.Driver.Options
					}
					opts.Labels = v.VolumeOptions.Labels
				}

				if _, err = cli.CreateVolume(ctx, machineID, opts); err != nil {
					return resp, fmt.Errorf("create volume '%s': %w", v.Name, err)
				}
			}
		}
	}

	deployment := cli.NewDeployment(spec, nil)
	plan, err := deployment.Run(ctx)
	if err != nil {
		return resp, err
	}

	resp.ID = plan.ServiceID
	resp.Name = plan.ServiceName

	return resp, err
}

// InspectService returns detailed information about a service and its containers.
// The nameOrID parameter can be either a service name or ID.
func (cli *Client) InspectService(ctx context.Context, nameOrID string) (api.Service, error) {
	var svc api.Service

	machines, err := cli.ListMachines(ctx, nil)
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

	// List all service containers including stopped ones.
	opts := container.ListOptions{All: true}
	machineContainers, err := cli.Docker.ListServiceContainers(listCtx, nameOrID, opts)
	if err != nil {
		return svc, fmt.Errorf("list containers: %w", err)
	}

	// Collect all containers on all machines that belong to the specified service.
	foundByID := false
	var containers []api.MachineServiceContainer
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
			// ListServiceContainers was proxied to only one machine.
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

		for _, ctr := range mc.Containers {
			if ctr.ServiceID() == nameOrID || ctr.ServiceName() == nameOrID {
				containers = append(containers, api.MachineServiceContainer{
					MachineID: machineID,
					Container: ctr,
				})

				if ctr.ServiceID() == nameOrID {
					foundByID = true
				}
			}
		}
	}

	if len(containers) == 0 {
		return svc, api.ErrNotFound
	}

	// Containers from different services may share the same service name (distributed and eventually consistent store
	// may not prevent this), or a service name might match another service's ID. In these cases, matching by ID takes
	// priority over matching by name.
	if foundByID {
		containers = slices.DeleteFunc(containers, func(mc api.MachineServiceContainer) bool {
			return mc.Container.ServiceID() != nameOrID
		})
	} else {
		// Matched only by name but there could be multiple services with the same name.
		serviceID := containers[0].Container.ServiceID()
		for _, mc := range containers[1:] {
			if mc.Container.ServiceID() != serviceID {
				return svc, fmt.Errorf("multiple services found with name '%s', use the service ID instead", nameOrID)
			}
		}
	}

	svc = api.Service{
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
func (cli *Client) InspectServiceFromStore(ctx context.Context, id string) (api.Service, error) {
	var svc api.Service

	resp, err := cli.MachineClient.InspectService(ctx, &pb.InspectServiceRequest{Id: id})
	if err != nil {
		if s, ok := status.FromError(err); ok {
			if s.Code() == codes.NotFound {
				return svc, api.ErrNotFound
			}
		}
		return svc, err
	}

	svc, err = api.ServiceFromProto(resp.Service)
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

	machines, err := cli.ListMachines(ctx, nil)
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
		wg.Go(func() {
			err := cli.StopContainer(ctx, svc.ID, mc.Container.ID, container.StopOptions{})
			if err != nil {
				errCh <- fmt.Errorf("stop container '%s': %w", mc.Container.ID, err)
				return
			}

			err = cli.RemoveContainer(ctx, svc.ID, mc.Container.ID, container.RemoveOptions{
				// Remove anonymous volumes created by the container.
				RemoveVolumes: true,
			})
			if err != nil && !errors.Is(err, api.ErrNotFound) {
				errCh <- fmt.Errorf("remove container '%s': %w", mc.Container.ID, err)
			}
		})
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

// StopService stops all containers on all machines that belong to the specified service.
// The id parameter can be either a service ID or name.
func (cli *Client) StopService(ctx context.Context, id string, opts container.StopOptions) error {
	svc, err := cli.InspectService(ctx, id)
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	errCh := make(chan error)

	// Stop all containers on all machines that belong to the service.
	for _, mc := range svc.Containers {
		wg.Go(func() {
			err := cli.StopContainer(ctx, svc.ID, mc.Container.ID, opts)
			if err != nil {
				errCh <- fmt.Errorf("stop container '%s': %w", mc.Container.ID, err)
			}
		})
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

// StartService starts all containers on all machines that belong to the specified service.
// The id parameter can be either a service ID or name.
func (cli *Client) StartService(ctx context.Context, id string) error {
	svc, err := cli.InspectService(ctx, id)
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	errCh := make(chan error)

	// Start all containers on all machines that belong to the service.
	for _, mc := range svc.Containers {
		wg.Go(func() {
			err := cli.StartContainer(ctx, svc.ID, mc.Container.ID)
			if err != nil {
				errCh <- fmt.Errorf("start container '%s': %w", mc.Container.ID, err)
			}
		})
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

// ListServices returns a list of all services and their containers.
func (cli *Client) ListServices(ctx context.Context) ([]api.Service, error) {
	machines, err := cli.ListMachines(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}

	// Broadcast the container list request to all available machines.
	md := metadata.New(nil)
	for _, m := range machines {
		if m.State == pb.MachineMember_UP || m.State == pb.MachineMember_SUSPECT {
			machineIP, _ := m.Machine.Network.ManagementIp.ToAddr()
			md.Append("machines", machineIP.String())
		}
		// TODO: warning about machines that are DOWN.
	}
	listCtx := metadata.NewOutgoingContext(ctx, md)

	// List all containers including stopped ones.
	opts := container.ListOptions{All: true}
	machineContainers, err := cli.Docker.ListServiceContainers(listCtx, "", opts)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	// TODO: optimise by extracting services from the list of all containers instead of inspecting each service.
	//  Most of the code can be reused in both InspectService and ListServices.
	servicesByID := make(map[string]api.Service)
	for _, mc := range machineContainers {
		if mc.Metadata != nil && mc.Metadata.Error != "" {
			// TODO: return failed machines in the response.
			fmt.Fprintf(os.Stderr, "WARNING: failed to list containers on machine '%s': %s\n",
				mc.Metadata.Machine, mc.Metadata.Error)
			continue
		}

		for _, ctr := range mc.Containers {
			if _, ok := servicesByID[ctr.ServiceID()]; ok {
				continue
			}

			svc, err := cli.InspectService(ctx, ctr.ServiceID())
			if err != nil {
				if errors.Is(err, api.ErrNotFound) {
					continue
				}
				return nil, fmt.Errorf("inspect service: %w", err)
			}

			servicesByID[ctr.ServiceID()] = svc
		}
	}

	services := make([]api.Service, 0, len(servicesByID))
	for _, svc := range servicesByID {
		services = append(services, svc)
	}
	return services, nil
}
