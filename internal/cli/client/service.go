package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/distribution/reference"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-connections/nat"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"slices"
	"strconv"
	"strings"
	"sync"
	"uncloud/internal/api"
	"uncloud/internal/machine/api/pb"
	machinedocker "uncloud/internal/machine/docker"
	"uncloud/internal/secret"
)

type RunServiceResponse struct {
	ID         string
	Name       string
	Containers []MachineContainerID
}

type MachineContainerID struct {
	MachineID   string
	ContainerID string
}

type Service struct {
	ID   string
	Name string
}

// NewService creates a new Service object with the specified name that can be used to run service containers.
// It doesn't create  anything in the cluster yet.
func (cli *Client) NewService(ctx context.Context, name string) (*Service, error) {
	// Optimistically check if a service with the specified name already exists.
	// TODO: introduce a distributed lock to hold for all service related operations.
	_, err := cli.InspectService(ctx, name)
	if err == nil {
		return nil, fmt.Errorf("service with name '%s' already exists", name)
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("inspect service: %w", err)
	}

	id, err := secret.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate service ID: %w", err)
	}

	return &Service{
		ID:   id,
		Name: name,
	}, nil
}

func (cli *Client) RunService(ctx context.Context, spec api.ServiceSpec) (RunServiceResponse, error) {
	var resp RunServiceResponse

	if err := spec.Validate(); err != nil {
		return resp, fmt.Errorf("invalid service spec: %w", err)
	}

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

	err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
		switch spec.Mode {
		case "", api.ServiceModeReplicated:
			resp, err = cli.runReplicatedService(ctx, serviceID, spec)
		case api.ServiceModeGlobal:
			resp, err = cli.runGlobalService(ctx, serviceID, spec)
		default:
			return fmt.Errorf("invalid mode: %q", spec.Mode)
		}

		return err
	}, cli.progressOut(), "Running service "+spec.Name)

	return resp, err
}

func (cli *Client) runReplicatedService(ctx context.Context, id string, spec api.ServiceSpec) (RunServiceResponse, error) {
	resp := RunServiceResponse{
		ID:   id,
		Name: spec.Name,
	}

	// Find a machine to run a service replica on.
	machines, err := cli.ListMachines(ctx)
	if err != nil {
		return resp, fmt.Errorf("list machines: %w", err)
	}

	// TODO: support selecting a particular machine by ID or name through the user options.
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
	//}

	m := firstAvailableMachine(machines)
	if m == nil {
		return resp, errors.New("no available machine to run the service")
	}

	runResp, err := cli.runContainer(ctx, id, spec, m.Machine)
	if err != nil {
		return resp, fmt.Errorf("run container: %w", err)
	}

	resp.Containers = append(resp.Containers, MachineContainerID{
		MachineID:   m.Machine.Id,
		ContainerID: runResp.ID,
	})

	return resp, nil
}

func firstAvailableMachine(machines []*pb.MachineMember) *pb.MachineMember {
	// Find the first UP machine.
	for _, m := range machines {
		if m.State == pb.MachineMember_UP {
			return m
		}
	}
	// There is no UP machine, try to find the first SUSPECT machine.
	for _, m := range machines {
		if m.State == pb.MachineMember_SUSPECT {
			return m
		}
	}

	return nil
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

	wg := sync.WaitGroup{}
	errCh := make(chan error)
	mu := sync.Mutex{}

	// Run a service container on each available machine.
	for _, m := range machines {
		if m.State != pb.MachineMember_UP && m.State != pb.MachineMember_SUSPECT {
			// TODO: return failed machines in the response.
			fmt.Printf("WARNING: failed to run a service container on machine '%s' which is Down.\n", m.Machine.Name)
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			runResp, err := cli.runContainer(ctx, id, spec, m.Machine)
			if err != nil {
				errCh <- fmt.Errorf("run container on machine '%s': %w", m.Machine.Name, err)
				return
			}

			mu.Lock()
			resp.Containers = append(resp.Containers, MachineContainerID{
				MachineID:   m.Machine.Id,
				ContainerID: runResp.ID,
			})
			mu.Unlock()
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

	return resp, err
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
			api.LabelServiceID:   serviceID,
			api.LabelServiceName: spec.Name,
			api.LabelManaged:     "",
		},
	}
	if spec.Mode == api.ServiceModeGlobal {
		config.Labels[api.LabelServiceMode] = api.ServiceModeGlobal
	}

	if len(spec.Ports) > 0 {
		encodedPorts := make([]string, len(spec.Ports))
		for i, p := range spec.Ports {
			encodedPorts[i], err = p.String()
			if err != nil {
				return resp, fmt.Errorf("encode service port spec: %w", err)
			}
		}

		config.Labels[api.LabelServicePorts] = strings.Join(encodedPorts, ",")
	}

	portBindings := make(nat.PortMap)
	for _, p := range spec.Ports {
		if p.Mode != api.PortModeHost {
			continue
		}
		port := nat.Port(fmt.Sprintf("%d/%s", p.ContainerPort, p.Protocol))
		portBindings[port] = []nat.PortBinding{
			{
				HostPort: strconv.Itoa(int(p.PublishedPort)),
			},
		}
		if p.HostIP.IsValid() {
			portBindings[port][0].HostIP = p.HostIP.String()
		}
	}
	hostConfig := &container.HostConfig{
		Binds:        spec.Container.Volumes,
		Init:         spec.Container.Init,
		PortBindings: portBindings,
	}
	netConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			machinedocker.NetworkName: {},
		},
	}

	pw := progress.ContextWriter(ctx)
	eventID := fmt.Sprintf("Container %s on %s", containerName, machine.Name)

	pw.Event(progress.CreatingEvent(eventID))
	resp, err = cli.CreateContainer(ctx, config, hostConfig, netConfig, nil, containerName)
	if err != nil {
		if !dockerclient.IsErrNotFound(err) {
			return resp, fmt.Errorf("create container: %w", err)
		}

		// Pull the missing image and create the container again.
		if err = cli.pullImageWithProgress(ctx, config.Image, machine.Name, eventID); err != nil {
			return resp, err
		}
		if resp, err = cli.CreateContainer(ctx, config, hostConfig, netConfig, nil, containerName); err != nil {
			return resp, fmt.Errorf("create container: %w", err)
		}
	}
	pw.Event(progress.CreatedEvent(eventID))

	pw.Event(progress.StartingEvent(eventID))
	if err = cli.StartContainer(ctx, resp.ID, container.StartOptions{}); err != nil {
		return resp, fmt.Errorf("start container: %w", err)
	}
	pw.Event(progress.StartedEvent(eventID))

	return resp, nil
}

func (cli *Client) pullImageWithProgress(ctx context.Context, image, machineName, parentEventID string) error {
	pw := progress.ContextWriter(ctx)
	eventID := fmt.Sprintf("Image %s on %s", image, machineName)
	pw.Event(progress.Event{
		ID:         eventID,
		ParentID:   parentEventID,
		Status:     progress.Working,
		StatusText: "Pulling",
	})

	pullCh, err := cli.PullImage(ctx, image)
	if err != nil {
		pw.Event(progress.Event{
			ID:         eventID,
			ParentID:   parentEventID,
			Text:       "Error",
			Status:     progress.Error,
			StatusText: errors.Unwrap(err).Error(),
		})
		return fmt.Errorf("pull image: %w", err)
	}

	// Wait for pull to complete by reading all progress messages and converting them to events.
	for msg := range pullCh {
		if msg.Err != nil {
			err = msg.Err
		} else {
			if msg.Message.Error != nil {
				err = errors.New(msg.Message.Error.Message)
			}
		}
		if err != nil {
			pw.Event(progress.Event{
				ID:         eventID,
				ParentID:   parentEventID,
				Text:       "Error",
				Status:     progress.Error,
				StatusText: errors.Unwrap(err).Error(),
			})
			return fmt.Errorf("pull image: %w", err)
		}

		// TODO: add like in compose: --quiet-pull Pull without printing progress information
		e := toPullProgressEvent(msg.Message)
		if e != nil {
			e.ID = fmt.Sprintf("%s on %s", e.ID, machineName)
			e.ParentID = eventID
			// Grand children events are not printed by the tty progress writer but they are still required
			// to calculate the progress line of their parent.
			pw.Event(*e)
		}
	}
	pw.Event(progress.Event{
		ID:         eventID,
		ParentID:   parentEventID,
		Status:     progress.Done,
		StatusText: "Pulled",
	})

	return nil
}

// toPullProgressEvent converts a JSON progress message from the Docker API to a progress event.
// It's based on toPullProgressEvent from Docker Compose.
func toPullProgressEvent(jm jsonmessage.JSONMessage) *progress.Event {
	if jm.ID == "" || jm.Progress == nil {
		return nil
	}

	var (
		total   int64
		percent int
		current int64
	)
	text := jm.Progress.String()
	stat := progress.Working

	switch jm.Status {
	case "Preparing", "Waiting", "Pulling fs layer":
		percent = 0
	case "Downloading", "Extracting", "Verifying Checksum":
		current = jm.Progress.Current
		total = jm.Progress.Total
		if jm.Progress.Total > 0 {
			percent = int(jm.Progress.Current * 100 / jm.Progress.Total)
		}
	case "Download complete", "Already exists", "Pull complete":
		stat = progress.Done
		percent = 100
	}

	if strings.Contains(jm.Status, "Image is up to date") ||
		strings.Contains(jm.Status, "Downloaded newer image") {
		stat = progress.Done
		percent = 100
	}

	return &progress.Event{
		ID:         jm.ID,
		Current:    current,
		Total:      total,
		Percent:    percent,
		Text:       jm.Status,
		Status:     stat,
		StatusText: text,
	}
}

// InspectService returns detailed information about a service and its containers.
// The id parameter can be either a service ID or name.
func (cli *Client) InspectService(ctx context.Context, id string) (api.Service, error) {
	var svc api.Service

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
			filters.Arg("label", api.LabelServiceID),
			filters.Arg("label", api.LabelManaged),
		),
	}
	machineContainers, err := cli.ListContainers(listCtx, opts)
	if err != nil {
		return svc, fmt.Errorf("list containers: %w", err)
	}

	// Collect all containers on all machines that belong to the specified service.
	foundByID := false
	var containers []api.MachineContainer
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
			ctr := api.Container{Container: c}
			if ctr.ServiceID() == id || ctr.ServiceName() == id {
				containers = append(containers, api.MachineContainer{
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
		containers = slices.DeleteFunc(containers, func(mc api.MachineContainer) bool {
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
				return svc, ErrNotFound
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

// ListServices returns a list of all services and their containers.
func (cli *Client) ListServices(ctx context.Context) ([]api.Service, error) {
	machines, err := cli.ListMachines(ctx)
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

	// List only uncloud-managed containers that belong to some service.
	opts := container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", api.LabelServiceID),
			filters.Arg("label", api.LabelManaged),
		),
	}
	machineContainers, err := cli.ListContainers(listCtx, opts)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	// TODO: optimise by extracting services from the list of all containers instead of inspecting each service.
	//  Most of the code can be reused in both InspectService and ListServices.
	servicesByID := make(map[string]api.Service)
	for _, mc := range machineContainers {
		if mc.Metadata != nil && mc.Metadata.Error != "" {
			// TODO: return failed machines in the response.
			fmt.Printf("WARNING: failed to list containers on machine '%s': %s\n",
				mc.Metadata.Machine, mc.Metadata.Error)
			continue
		}

		for _, c := range mc.Containers {
			ctr := api.Container{Container: c}
			if _, ok := servicesByID[ctr.ServiceID()]; ok {
				continue
			}

			svc, err := cli.InspectService(ctx, ctr.ServiceID())
			if err != nil {
				if errors.Is(err, ErrNotFound) {
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
