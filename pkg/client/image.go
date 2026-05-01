package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
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

// This is the container image used to run socat proxy containers.
// TODO: it's an external dependency that we don't control, so we should consider vendoring it or
// automatically building an ad-hoc image with socat-like functionality (e.g. using a lightweight Go
// proxy) as part of the push process.
const socatImage = "alpine/socat:1.8.0.3"

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

	dockerEnv, err := detectDockerEnvironment(ctx, dockerCli)
	if err != nil {
		return err
	}
	slog.Debug("Detected Docker environment:", "virtualised", dockerEnv.Virtualised, "rootless", dockerEnv.Rootless)

	proxyEventID := fmt.Sprintf("Proxy to unregistry on %s", boldStyle.Render(machine.Name))
	pw.Event(progress.StartingEvent(proxyEventID))

	// The proxy runs in a goroutine. Capture the first error in a channel
	// so we can surface it alongside the push error if push fails.
	proxyErrCh := make(chan error, 1)
	onProxyError := func(err error) {
		select {
		case proxyErrCh <- fmt.Errorf("proxy to unregistry: %w", err):
		default:
		}
		pw.Event(progress.NewEvent(proxyEventID, progress.Error, err.Error()))
	}

	// socketPath is set for plain rootless Docker (not running inside a VM): the Go proxy listens on a unix
	// socket that is bind-mounted into the socat container, bypassing slirp4netns network routing entirely.
	var (
		socketPath string
		proxyPort  int
	)
	var unregProxy *proxy.Proxy
	if shouldUseUnregistryUnixProxy(dockerEnv) {
		suffix, err := secret.RandomAlphaNumeric(4)
		if err != nil {
			pw.Event(progress.NewEvent(proxyEventID, progress.Error, err.Error()))
			return fmt.Errorf("generate socket path suffix: %w", err)
		}
		socketPath = filepath.Join(os.TempDir(), fmt.Sprintf("uncloud-push-%s.sock", suffix))
		unregProxy, err = newUnregistryUnixProxy(ctx, unregistryAddr, dialer, socketPath, onProxyError)
		if err != nil {
			pw.Event(progress.NewEvent(proxyEventID, progress.Error, err.Error()))
			return fmt.Errorf("create local unix socket proxy to unregistry on machine '%s': %w", machine.Name, err)
		}
		slog.Debug("Listening for unregistry proxy connections.", "mode", "unix", "socket", socketPath)
	} else {
		unregProxy, err = newUnregistryTcpProxy(ctx, unregistryAddr, dialer, onProxyError)
		if err != nil {
			pw.Event(progress.NewEvent(proxyEventID, progress.Error, err.Error()))
			return fmt.Errorf("create local tcp proxy to unregistry on machine '%s': %w", machine.Name, err)
		}
		proxyPort = unregProxy.Listener.Addr().(*net.TCPAddr).Port
		slog.Debug("Listening for unregistry proxy connections.", "mode", "tcp", "port", proxyPort)
	}

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

		// Remove the unix socket file used for rootless Docker proxying.
		if socketPath != "" {
			os.Remove(socketPath)
		}

		cancelProxy()
	}
	defer cleanup()

	go unregProxy.Run(proxyCtx)

	if dockerEnv.Virtualised {
		// VM-based Docker (Docker Desktop, Rancher Desktop, etc.): run a socat container inside the VM
		// to forward a port via host.docker.internal back to the host-side proxy.
		pw.Event(progress.Event{
			ID:         proxyEventID,
			Status:     progress.Working,
			StatusText: "Starting",
			Text:       "(detected Docker in VM locally, starting socat proxy container)",
		})
		proxyCtrID, proxyPort, err = runDockerVMProxyContainer(ctx, dockerCli, proxyPort)
		if err != nil {
			pw.Event(progress.NewEvent(proxyEventID, progress.Error, err.Error()))
			return fmt.Errorf("run socat container to proxy unregistry: %w", err)
		}
		slog.Debug("Started VM socat proxy container.", "id", proxyCtrID, "hostPort", proxyPort)
	} else if shouldUseUnregistryUnixProxy(dockerEnv) {
		// Plain rootless Docker: run a socat container that forwards via a bind-mounted unix socket,
		// bypassing the slirp4netns --disable-host-loopback restriction.
		pw.Event(progress.Event{
			ID:         proxyEventID,
			Status:     progress.Working,
			StatusText: "Starting",
			Text:       "(detected rootless Docker, starting socat proxy container)",
		})
		proxyCtrID, proxyPort, err = runUnixSocketProxyContainer(ctx, dockerCli, socketPath)
		if err != nil {
			pw.Event(progress.NewEvent(proxyEventID, progress.Error, err.Error()))
			return fmt.Errorf("run socat container with unix socket to proxy unregistry: %w", err)
		}
		slog.Debug("Started unix socket socat proxy container.", "id", proxyCtrID, "hostPort", proxyPort, "socket", socketPath)
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
			// Include the proxy error (if any) to expose the root cause behind a generic push failure.
			select {
			case proxyErr := <-proxyErrCh:
				return fmt.Errorf("push image: %w", errors.Join(msg.Err, proxyErr))
			default:
			}

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

// checkRemoteConnectivity verifies that remoteAddr is reachable via dialer within a timeout.
func checkRemoteConnectivity(ctx context.Context, dialer netproxy.ContextDialer, remoteAddr string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", remoteAddr)
	if err != nil {
		return fmt.Errorf("connect to remote address '%s': %w", remoteAddr, err)
	}
	conn.Close()
	return nil
}

// newUnregistryTcpProxy creates a TCP proxy that listens on localhost and forwards to the unregistry
// address on the target machine.
func newUnregistryTcpProxy(
	ctx context.Context, remoteAddr string, dialer netproxy.ContextDialer, onError func(error),
) (*proxy.Proxy, error) {
	if err := checkRemoteConnectivity(ctx, dialer, remoteAddr); err != nil {
		return nil, err
	}

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

// newUnregistryUnixProxy creates a proxy that listens on a unix socket at socketPath and forwards to the
// unregistry address on the target machine.
func newUnregistryUnixProxy(
	ctx context.Context, remoteAddr string, dialer netproxy.ContextDialer, socketPath string, onError func(error),
) (*proxy.Proxy, error) {
	if err := checkRemoteConnectivity(ctx, dialer, remoteAddr); err != nil {
		return nil, err
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on unix socket %s: %w", socketPath, err)
	}
	// Restrict socket access to the current user to prevent other local users from connecting
	// to the proxy and reaching the unregistry for the duration of the push.
	if err = os.Chmod(socketPath, 0o600); err != nil {
		listener.Close()
		return nil, fmt.Errorf("set unix socket permissions on %s: %w", socketPath, err)
	}

	return &proxy.Proxy{
		Listener:    listener,
		RemoteAddr:  remoteAddr,
		DialContext: dialer.DialContext,
		OnError:     onError,
	}, nil
}

// dockerEnvironment describes the local Docker environment type.
type dockerEnvironment struct {
	// Virtualised indicates Docker runs inside a VM (Docker Desktop, Rancher Desktop, Colima).
	Virtualised bool
	// Rootless indicates Docker is running in rootless mode
	Rootless bool
}

// detectDockerEnvironment inspects the local Docker daemon to determine if it is running in a virtualised
// environment (VM) or in rootless mode. Both cases require a socat proxy container to route connections
// across the network boundary between the local Docker daemon and the host.
func detectDockerEnvironment(ctx context.Context, dockerCli *docker.Client) (dockerEnvironment, error) {
	info, err := dockerCli.Info(ctx)
	if err != nil {
		return dockerEnvironment{}, fmt.Errorf("get Docker info: %w", err)
	}

	env := dockerEnvironment{}

	for _, opt := range info.SecurityOptions {
		if strings.Contains(opt, "rootless") {
			env.Rootless = true
			break
		}
	}

	// On macOS, Docker always requires a VM, so set Virtualised to true unless OrbStack is
	// detected (which handles host networking natively).
	if runtime.GOOS == "darwin" {
		if info.Name != "orbstack" {
			env.Virtualised = true
		}
		return env, nil
	}

	// On other platforms, check for known virtualised Docker environments.
	virtualisedHostnames := []string{"docker-desktop", "rancher-desktop", "colima"}
	for _, name := range virtualisedHostnames {
		if strings.Contains(strings.ToLower(info.Name), name) {
			env.Virtualised = true
			break
		}
	}
	return env, nil
}

// shouldUseUnregistryUnixProxy reports whether the local Docker environment requires a unix socket proxy
// to reach the unregistry. This is the case for rootless Docker not running inside a VM, where
// slirp4netns --disable-host-loopback blocks TCP routing from the container network namespace back to the host.
func shouldUseUnregistryUnixProxy(env dockerEnvironment) bool {
	// TODO: handle Virtualised AND Rootless case when we encounter it.
	return env.Rootless && !env.Virtualised
}

// runDockerVMProxyContainer creates a socat container inside the Docker VM (e.g. Docker Desktop on macOS)
// to forward TCP connections from a localhost port to the specified target port on the host via host.docker.internal.
// Returns the container ID and the localhost port the container port is bound to.
func runDockerVMProxyContainer(ctx context.Context, dockerCli *docker.Client, targetPort int) (string, int, error) {
	return runSocatProxyContainer(ctx, dockerCli,
		fmt.Sprintf("TCP-CONNECT:host.docker.internal:%d", targetPort), nil)
}

// runUnixSocketProxyContainer creates a socat container that forwards TCP connections to the host-side Go
// proxy via a bind-mounted unix socket.
// Returns the container ID and the localhost port that 'docker push' should target.
func runUnixSocketProxyContainer(ctx context.Context, dockerCli *docker.Client, socketPath string) (string, int, error) {
	return runSocatProxyContainer(ctx, dockerCli,
		fmt.Sprintf("UNIX-CONNECT:%s", socketPath),
		[]string{fmt.Sprintf("%s:%s", socketPath, socketPath)})
}

// runSocatProxyContainer creates a socat container that listens on TCP port 5000 and forwards to socatDst.
// binds is an optional list of host:container bind mounts (e.g. for a unix socket).
// Returns the container ID and the localhost port that clients should connect to.
func runSocatProxyContainer(ctx context.Context, dockerCli *docker.Client, socatDst string, binds []string) (string, int, error) {
	suffix, err := secret.RandomAlphaNumeric(4)
	if err != nil {
		return "", 0, fmt.Errorf("generate random suffix: %w", err)
	}
	containerName := fmt.Sprintf("uncloud-push-proxy-%s", suffix)

	containerPort := nat.Port("5000/tcp")
	config := &container.Config{
		// TODO: make image configurable.
		Image: socatImage,
		// Reset the default entrypoint "socat".
		Entrypoint: []string{},
		Cmd: []string{
			"timeout", "1800", // Auto-terminate socat after 30 minutes.
			"socat",
			"TCP-LISTEN:5000,fork,reuseaddr",
			socatDst,
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
		Binds:      binds,
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

	// Wait for socat to start listening inside the container. ContainerStart returns as soon as the
	// container process is launched, but socat may not have bound its port yet. Without this check,
	// the first 'docker push' connection can arrive before socat is ready and get reset.
	addr := fmt.Sprintf("127.0.0.1:%d", hostPort)
	if err = waitForTCPPort(addr, 10*time.Second); err != nil {
		cleanup()
		return "", 0, fmt.Errorf("socat proxy container %s: %w", resp.ID, err)
	}

	return resp.ID, hostPort, nil
}

// waitForTCPPort polls addr until a TCP connection succeeds or timeout is reached.
func waitForTCPPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("port %s did not become ready within %s", addr, timeout)
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
		percent = min(
			// Cap percent at 100 to prevent index out of bounds in progress display.
			// Docker can report Current > Total in some cases (e.g., compression).
			int(jm.Progress.Current*100/jm.Progress.Total), 100)
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
