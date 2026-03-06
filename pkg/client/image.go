package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/containerd/errdefs"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-connections/nat"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/psviderski/uncloud/internal/docker"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/constants"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/psviderski/uncloud/internal/proxy"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/api"
	netproxy "golang.org/x/net/proxy"
)

func (cli *Client) InspectImage(ctx context.Context, id string) ([]api.MachineImage, error) {
	images, err := cli.Docker.InspectImage(ctx, id)
	if errdefs.IsNotFound(err) {
		err = api.ErrNotFound
	}

	return images, err
}

func (cli *Client) InspectRemoteImage(ctx context.Context, id string) ([]api.MachineRemoteImage, error) {
	return cli.Docker.InspectRemoteImage(ctx, id)
}

// ListImages returns a list of images on specified machines in the cluster. If no machines are specified in the filter,
// it lists images on all machines.
func (cli *Client) ListImages(ctx context.Context, filter api.ImageFilter) ([]api.MachineImages, error) {
	// Broadcast the image list request to the specified machines or all machines if none specified.
	listCtx, machines, err := cli.ProxyMachinesContext(ctx, filter.Machines)
	if err != nil {
		return nil, fmt.Errorf("create request context to broadcast to machines: %w", err)
	}

	opts := image.ListOptions{Manifests: true}
	if filter.Name != "" {
		opts.Filters = filters.NewArgs(
			filters.Arg("reference", filter.Name),
		)
	}

	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("marshal options: %w", err)
	}

	resp, err := cli.Docker.GRPCClient.ListImages(listCtx, &pb.ListImagesRequest{Options: optsBytes})
	if err != nil {
		return nil, err
	}

	machineImages := make([]api.MachineImages, len(resp.Messages))
	for i, msg := range resp.Messages {
		machineImages[i].Metadata = msg.Metadata
		// TODO: handle this in the grpc-proxy router and always provide Metadata if possible.
		if msg.Metadata == nil {
			// Metadata can be nil if the request was broadcasted to only one machine.
			machineImages[i].Metadata = &pb.Metadata{
				Machine: machines[0].Machine.Id,
			}
		} else {
			// Replace management IP with machine ID for friendlier error messages.
			// TODO: migrate Metadata.Machine to use machine ID instead of IP in the grpc-proxy router.
			if m := machines.FindByManagementIP(msg.Metadata.Machine); m != nil {
				machineImages[i].Metadata.Machine = m.Machine.Id
			}
			if msg.Metadata.Error != "" {
				continue
			}
		}

		if len(msg.Images) > 0 {
			if err = json.Unmarshal(msg.Images, &machineImages[i].Images); err != nil {
				return nil, fmt.Errorf("unmarshal images: %w", err)
			}
		}
		machineImages[i].ContainerdStore = msg.ContainerdStore
	}

	return machineImages, nil
}

// RemoveImage removes an image from the specified machines.
func (cli *Client) RemoveImage(
	ctx context.Context, image string, opts image.RemoveOptions, machines []string,
) ([]api.MachineRemoveImageResponse, error) {
	listCtx, _, err := cli.ProxyMachinesContext(ctx, machines)
	if err != nil {
		return nil, fmt.Errorf("create request context to broadcast to machines: %w", err)
	}

	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("marshal options: %w", err)
	}

	resp, err := cli.Docker.GRPCClient.RemoveImage(listCtx, &pb.RemoveImageRequest{
		Image:   image,
		Options: optsBytes,
	})
	if err != nil {
		return nil, err
	}

	machineResponses := make([]api.MachineRemoveImageResponse, len(resp.Messages))
	for i, msg := range resp.Messages {
		machineResponses[i].Metadata = msg.Metadata
		if msg.Metadata.Error != "" {
			continue
		}

		if len(msg.Response) > 0 {
			if err = json.Unmarshal(msg.Response, &machineResponses[i].Response); err != nil {
				return nil, fmt.Errorf("unmarshal response: %w", err)
			}
		}
	}

	return machineResponses, nil
}

// PullImage pulls an image on the specified machines.
func (cli *Client) PullImage(
	ctx context.Context, imageName string, opts image.PullOptions, machines []string,
) (<-chan api.MachinePullImageMessage, error) {
	listCtx, _, err := cli.ProxyMachinesContext(ctx, machines)
	if err != nil {
		return nil, fmt.Errorf("create request context to broadcast to machines: %w", err)
	}

	if opts.RegistryAuth == "" {
		// Try to retrieve the authentication token for the image from the default local Docker config file.
		if encodedAuth, err := docker.RetrieveLocalDockerRegistryAuth(imageName); err == nil {
			opts.RegistryAuth = encodedAuth
		}
	}

	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("marshal options: %w", err)
	}

	stream, err := cli.Docker.GRPCClient.PullImage(listCtx, &pb.PullImageRequest{
		Image:   imageName,
		Options: optsBytes,
	})
	if err != nil {
		return nil, err
	}

	ch := make(chan api.MachinePullImageMessage)
	go func() {
		defer close(ch)
		for {
			msg, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				ch <- api.MachinePullImageMessage{Err: err}
				return
			}

			var jm jsonmessage.JSONMessage
			if err := json.Unmarshal(msg.Message, &jm); err != nil {
				ch <- api.MachinePullImageMessage{Err: fmt.Errorf("unmarshal message: %w", err)}
				return
			}

			pullMsg := api.MachinePullImageMessage{Message: jm}
			if jm.Error != nil {
				pullMsg.Err = errors.New(jm.Error.Message)
			}

			select {
			case <-ctx.Done():
				ch <- api.MachinePullImageMessage{Err: ctx.Err()}
				return
			case ch <- pullMsg:
			}
		}
	}()

	return ch, nil
}

// PruneImages prunes unused images on the specified machines.
func (cli *Client) PruneImages(
	ctx context.Context, filtersArgs filters.Args, machines []string,
) ([]api.MachinePruneImagesResponse, error) {
	listCtx, _, err := cli.ProxyMachinesContext(ctx, machines)
	if err != nil {
		return nil, fmt.Errorf("create request context to broadcast to machines: %w", err)
	}

	filtersBytes, err := json.Marshal(filtersArgs)
	if err != nil {
		return nil, fmt.Errorf("marshal filters: %w", err)
	}

	resp, err := cli.Docker.GRPCClient.PruneImages(listCtx, &pb.PruneImagesRequest{
		Filters: filtersBytes,
	})
	if err != nil {
		return nil, err
	}

	machineResponses := make([]api.MachinePruneImagesResponse, len(resp.Messages))
	for i, msg := range resp.Messages {
		machineResponses[i].Metadata = msg.Metadata
		if msg.Metadata.Error != "" {
			continue
		}

		if len(msg.Report) > 0 {
			if err = json.Unmarshal(msg.Report, &machineResponses[i].Report); err != nil {
				return nil, fmt.Errorf("unmarshal report: %w", err)
			}
		}
	}

	return machineResponses, nil
}

// TagImage creates a tag for an image on the specified machines.
func (cli *Client) TagImage(ctx context.Context, source, target string, machines []string) error {
	listCtx, _, err := cli.ProxyMachinesContext(ctx, machines)
	if err != nil {
		return fmt.Errorf("create request context to broadcast to machines: %w", err)
	}

	_, err = cli.Docker.GRPCClient.TagImage(listCtx, &pb.TagImageRequest{
		Source: source,
		Target: target,
	})
	return err
}

type PushImageOptions struct {
	// AllMachines pushes the image to all machines in the cluster. Takes precedence over Machines field.
	AllMachines bool
	// Machines is a list of machine names or IDs to push the image to. If empty and AllMachines is false,
	// pushes to the machine the client is connected to.
	Machines []string
	// Platform to push for a multi-platform image. Local Docker must use containerd image store
	// to support multi-platform images.
	Platform *ocispec.Platform
}

// PushImage pushes a local Docker image to the specified machines. If no machines are specified,
// it pushes to the machine the client is connected to.
func (cli *Client) PushImage(ctx context.Context, image string, opts PushImageOptions) error {
	dockerCliWrapped, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("create Docker client: %w", err)
	}
	dockerCli := &docker.Client{Client: dockerCliWrapped}
	defer dockerCli.Close()

	// Check if Docker image exists locally.
	if _, err = dockerCli.ImageInspect(ctx, image); err != nil {
		if errdefs.IsNotFound(err) {
			return fmt.Errorf("image '%s' not found locally", image)
		}
		return fmt.Errorf("inspect image '%s' locally: %w", image, err)
	}

	// Get the machine info for the specified machines or the connected machine if none are specified.
	var machines []*pb.MachineInfo
	if opts.AllMachines {
		machineMembers, err := cli.ListMachines(ctx, nil)
		if err != nil {
			return fmt.Errorf("list machines: %w", err)
		}
		for _, mm := range machineMembers {
			machines = append(machines, mm.Machine)
		}
	} else if len(opts.Machines) > 0 {
		machineMembers, err := cli.ListMachines(ctx, &api.MachineFilter{
			NamesOrIDs: opts.Machines,
		})
		if err != nil {
			return fmt.Errorf("list machines: %w", err)
		}

		for _, mm := range machineMembers {
			machines = append(machines, mm.Machine)
		}
	} else {
		// No machines specified, use the connected machine.
		m, err := cli.MachineClient.Inspect(ctx, nil)
		if err != nil {
			return fmt.Errorf("inspect connected machine: %w", err)
		}

		// If the machine has been renamed, the new name will only be stored in the cluster store. .Inspect will return
		// the old name from the machine config. So we need to fetch the machine info from the cluster.
		// TODO: make one source of truth for machine info.
		mm, err := cli.InspectMachine(ctx, m.Id)
		if err != nil {
			return fmt.Errorf("inspect machine: %w", err)
		}
		machines = append(machines, mm.Machine)
	}

	// Push image to all specified machines.
	var wg sync.WaitGroup
	errCh := make(chan error, len(machines))

	// TODO: detect the target machine platform and figure out how to handle scenarios when local and target
	//  platforms differ.
	for _, m := range machines {
		wg.Go(func() {
			if err := cli.pushImageToMachine(ctx, dockerCli, image, m, opts.Platform); err != nil {
				errCh <- fmt.Errorf("push image to machine '%s': %w", m.Name, err)
			}
		})
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err = range errCh {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// pushImageToMachine pushes a local Docker image to a specific machine using local port forwarding to its unregistry.
func (cli *Client) pushImageToMachine(
	ctx context.Context,
	dockerCli *docker.Client,
	imageName string,
	machine *pb.MachineInfo,
	platform *ocispec.Platform,
) error {
	pw := progress.ContextWriter(ctx)
	boldStyle := lipgloss.NewStyle().Bold(true)
	pushEventID := fmt.Sprintf("Pushing %s to %s", boldStyle.Render(imageName), boldStyle.Render(machine.Name))

	// Check the Docker image store type on the target machine.
	images, err := cli.ListImages(ctx, api.ImageFilter{
		Machines: []string{machine.Id},
		Name:     "%invalid-name-to-only-check-store-type%",
	})
	if err != nil {
		return fmt.Errorf("check Docker image store type on machine '%s': %w", machine.Name, err)
	}

	// Only support Docker with containerd image store enabled to avoid the confusion of pushing images to containerd
	// and then not being able to see and use them in Docker.
	if !images[0].ContainerdStore {
		pw.Event(progress.NewEvent(pushEventID, progress.Error, "containerd image store required"))
		return fmt.Errorf("docker on machine '%s' is not using containerd image store, "+
			"which is required for pushing images. Follow the instructions to enable it: "+
			"https://docs.docker.com/engine/storage/containerd/, and then restart the uncloud daemon "+
			"via 'systemctl restart uncloud'", machine.Name)
	}

	machineSubnet, _ := machine.Network.Subnet.ToPrefix()
	machineIP := network.MachineIP(machineSubnet)
	unregistryAddr := net.JoinHostPort(machineIP.String(), strconv.Itoa(constants.UnregistryPort))

	dialer, err := cli.connector.Dialer()
	if err != nil {
		return fmt.Errorf("get proxy dialer: %w", err)
	}

	proxyEventID := fmt.Sprintf("Proxy to unregistry on %s", boldStyle.Render(machine.Name))
	pw.Event(progress.StartingEvent(proxyEventID))

	// Forward local port 127.0.0.1:PORT to the machine's unregistry over the established client connection.
	unregProxy, err := newUnregistryProxy(ctx, unregistryAddr, dialer, func(err error) {
		pw.Event(progress.NewEvent(proxyEventID, progress.Error, err.Error()))
	})
	if err != nil {
		pw.Event(progress.NewEvent(proxyEventID, progress.Error, err.Error()))
		return fmt.Errorf("create local proxy to unregistry on machine '%s': %w", machine.Name, err)
	}
	// Get the local port the unregistry proxy is listening on.
	proxyPort := unregProxy.Listener.Addr().(*net.TCPAddr).Port

	proxyCtx, cancelProxy := context.WithCancel(ctx)
	proxyCtrID := ""
	pushImageTag := ""

	// Cleanup function to remove temporary resources and stop proxies.
	cleanup := func() {
		// Remove temporary image tag.
		if pushImageTag != "" {
			dockerCli.ImageRemove(ctx, pushImageTag, image.RemoveOptions{})
		}

		// Remove socat proxy container.
		if proxyCtrID != "" {
			dockerCli.ContainerRemove(ctx, proxyCtrID, container.RemoveOptions{Force: true})
		}

		cancelProxy()
	}
	defer cleanup()

	go unregProxy.Run(proxyCtx)

	dockerVirtualised, err := isDockerVirtualised(ctx, dockerCli)
	if err != nil {
		return err
	}

	if dockerVirtualised {
		// Run socat proxy container to forward a localhost port from within the Docker VM to the host machine.
		pw.Event(progress.Event{
			ID:         proxyEventID,
			Status:     progress.Working,
			StatusText: "Starting",
			Text:       "(detected Docker in VM locally, starting socat proxy container)",
		})

		proxyCtrID, proxyPort, err = runDockerVMProxyContainer(ctx, dockerCli, proxyPort)
		if err != nil {
			pw.Event(progress.NewEvent(proxyEventID, progress.Error, err.Error()))
			return fmt.Errorf("run socat container to proxy unregistry to Docker VM: %w", err)
		}
	}

	pw.Event(progress.Event{
		ID:         proxyEventID,
		Status:     progress.Done,
		StatusText: "Started",
		Text:       fmt.Sprintf("(localhost:%d → %s)", proxyPort, unregistryAddr),
	})

	// Tag the image for pushing through the proxy.
	pushImageTag = fmt.Sprintf("127.0.0.1:%d/%s", proxyPort, imageName)
	if err = dockerCli.ImageTag(ctx, imageName, pushImageTag); err != nil {
		return fmt.Errorf("tag image for push: %w", err)
	}

	// Push the image through the proxy.
	pw.Event(progress.NewEvent(pushEventID, progress.Working, "Pushing"))

	pushCh, err := dockerCli.PushImage(ctx, pushImageTag, image.PushOptions{
		Platform: platform,
	})
	if err != nil {
		pw.Event(progress.NewEvent(pushEventID, progress.Error, err.Error()))
		return fmt.Errorf("push image: %w", err)
	}

	// Wait for push to complete by reading all progress messages and converting them to events.
	// If the context is cancelled, the pushCh will receive a context cancellation error.
	for msg := range pushCh {
		if msg.Err != nil {
			pw.Event(progress.NewEvent(pushEventID, progress.Error, msg.Err.Error()))
			return fmt.Errorf("push image: %w", msg.Err)
		}

		// TODO: support quite mode like in compose: --quiet Push without printing progress information
		if e := toPushProgressEvent(msg.Message); e != nil {
			e.ID = fmt.Sprintf("Layer %s on %s:", e.ID, boldStyle.Render(machine.Name))
			e.ParentID = pushEventID
			pw.Event(*e)
		}
	}
	pw.Event(progress.NewEvent(pushEventID, progress.Done, "Pushed"))

	return nil
}

func newUnregistryProxy(
	ctx context.Context, remoteAddr string, dialer netproxy.ContextDialer, onError func(error),
) (*proxy.Proxy, error) {
	// Test remote connectivity before creating a proxy.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	testConn, err := dialer.DialContext(ctx, "tcp", remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("connect to remote address '%s': %w", remoteAddr, err)
	}
	testConn.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen on an available port on 127.0.0.1: %w", err)
	}

	p := &proxy.Proxy{
		Listener:    listener,
		RemoteAddr:  remoteAddr,
		DialContext: dialer.DialContext,
		OnError:     onError,
	}

	return p, nil
}

// isDockerVirtualised checks if Docker is running in a virtualised environment like Docker/Rancher Desktop or Colima.
// On macOS, Docker always requires a VM, so it returns true unless OrbStack is detected (which handles host networking
// natively). On other platforms, it checks for known virtualised Docker hostnames.
func isDockerVirtualised(ctx context.Context, dockerCli *docker.Client) (bool, error) {
	info, err := dockerCli.Info(ctx)
	if err != nil {
		return false, fmt.Errorf("get Docker info: %w", err)
	}

	// On macOS, Docker always runs in a VM. OrbStack is the only known exception that doesn't need a proxy.
	if runtime.GOOS == "darwin" {
		if info.Name == "orbstack" {
			return false, nil
		}
		return true, nil
	}

	// On other platforms, check for known virtualised Docker environments.
	virtualisedHostnames := []string{"docker-desktop", "rancher-desktop", "colima"}
	for _, name := range virtualisedHostnames {
		if strings.Contains(strings.ToLower(info.Name), name) {
			return true, nil
		}
	}

	return false, nil
}

// runDockerVMProxyContainer creates a socat container to proxy an available localhost port within the Docker VM
// (e.g. Docker Desktop on macOS) to the specified target port on the host machine.
// Returns the container ID and the localhost port the container port is bound to.
// TODO: accept custom image name.
func runDockerVMProxyContainer(ctx context.Context, dockerCli *docker.Client, targetPort int) (string, int, error) {
	suffix, err := secret.RandomAlphaNumeric(4)
	if err != nil {
		return "", 0, fmt.Errorf("generate random suffix: %w", err)
	}
	containerName := fmt.Sprintf("uncloud-push-proxy-%s", suffix)

	containerPort := nat.Port("5000/tcp")
	config := &container.Config{
		// TODO: make image configurable.
		Image: "alpine/socat:latest",
		// Reset the default entrypoint "socat".
		Entrypoint: []string{},
		Cmd: []string{
			"timeout", "1800", // Auto-terminate socat after 30 minutes.
			"socat",
			"TCP-LISTEN:5000,fork,reuseaddr",
			fmt.Sprintf("TCP-CONNECT:host.docker.internal:%d", targetPort),
		},
		ExposedPorts: nat.PortSet{
			containerPort: {},
		},
		Labels: map[string]string{
			api.LabelManaged: "",
		},
	}

	// Get an available port on localhost to bind the container port to by creating a temporary listener and closing it.
	// We need to explicitly specify the host port and not rely on Docker mapping because if not specified,
	// 'docker push' from Docker Desktop is unable to reach the randomly mapped one for some reason.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", 0, fmt.Errorf("reserve a local port: %w", err)
	}
	hostPort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	hostConfig := &container.HostConfig{
		AutoRemove: true,
		PortBindings: nat.PortMap{
			containerPort: []nat.PortBinding{
				{
					HostIP:   "127.0.0.1",
					HostPort: strconv.Itoa(hostPort),
				},
			},
		},
	}

	resp, err := dockerCli.CreateContainerWithImagePull(ctx, containerName, config, hostConfig)
	if err != nil {
		return "", 0, fmt.Errorf("create socat proxy container: %w", err)
	}

	cleanup := func() {
		dockerCli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
	}

	if err = dockerCli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Clean up if start fails.
		cleanup()
		return "", 0, fmt.Errorf("start socat proxy container %s: %w", resp.ID, err)
	}

	return resp.ID, hostPort, nil
}

// toPushProgressEvent converts a JSON progress message from the Docker API to a progress event.
// It's based on toPushProgressEvent from Docker Compose.
func toPushProgressEvent(jm jsonmessage.JSONMessage) *progress.Event {
	if jm.ID == "" || jm.Progress == nil {
		return nil
	}

	status := progress.Working
	percent := 0

	if jm.Progress.Total > 0 {
		percent = int(jm.Progress.Current * 100 / jm.Progress.Total)
		// Cap percent at 100 to prevent index out of bounds in progress display.
		// Docker can report Current > Total in some cases (e.g., compression).
		if percent > 100 {
			percent = 100
		}
	}

	switch jm.Status {
	case "Pushed", "Layer already exists":
		status = progress.Done
		percent = 100
	}

	return &progress.Event{
		ID:         jm.ID,
		Current:    jm.Progress.Current,
		Total:      jm.Progress.Total,
		Percent:    percent,
		Text:       jm.Status,
		Status:     status,
		StatusText: jm.Progress.String(),
	}
}
