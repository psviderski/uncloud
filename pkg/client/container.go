package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/psviderski/uncloud/internal/docker"
	machinedocker "github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/api"
	"google.golang.org/grpc/status"
)

// TODO: format container and machine IDs in 'Container %s on %s' events as bold.
//  Consider formatting containers as <service_name>/<short-container-id>.

// CreateContainer creates a new container for the given service on the specified machine.
func (cli *Client) CreateContainer(
	ctx context.Context, serviceID string, spec api.ServiceSpec, machineID string,
) (container.CreateResponse, error) {
	var resp container.CreateResponse

	spec = spec.SetDefaults()
	if err := spec.Validate(); err != nil {
		return resp, fmt.Errorf("invalid service spec: %w", err)
	}
	// TODO: validate spec.Name is consistent with serviceID if this is not the first container in the service.

	machine, err := cli.InspectMachine(ctx, machineID)
	if err != nil {
		return resp, fmt.Errorf("inspect machine '%s': %w", machineID, err)
	}

	suffix, err := secret.RandomAlphaNumeric(4)
	if err != nil {
		return resp, fmt.Errorf("generate random suffix: %w", err)
	}
	containerName := fmt.Sprintf("%s-%s", spec.Name, suffix)

	// Proxy Docker gRPC requests to the selected machine.
	ctx = cli.ProxyMachineContext(ctx, machine.Machine.Id)

	pw := progress.ContextWriter(ctx)
	eventID := fmt.Sprintf("Container %s on %s", containerName, machine.Machine.Name)
	pw.Event(progress.CreatingEvent(eventID))

	if spec.Container.PullPolicy == api.PullPolicyAlways {
		if err = cli.pullImageWithProgress(ctx, spec.Container.Image, machine.Machine.Name, eventID); err != nil {
			return resp, err
		}
	}

	resp, err = cli.Docker.CreateServiceContainer(ctx, serviceID, spec, containerName)
	if err != nil {
		switch spec.Container.PullPolicy {
		case api.PullPolicyAlways, api.PullPolicyNever:
			return resp, err
		case api.PullPolicyMissing:
		default:
			return resp, fmt.Errorf("unsupported pull policy: '%s'", spec.Container.PullPolicy)
		}

		// NotFound (No such image) error is expected if the image is missing.
		if !errdefs.IsNotFound(err) || !strings.Contains(err.Error(), "No such image") {
			return resp, err
		}

		// Pull the missing image and create the container again.
		if err = cli.pullImageWithProgress(ctx, spec.Container.Image, machine.Machine.Name, eventID); err != nil {
			return resp, err
		}
		if resp, err = cli.Docker.CreateServiceContainer(ctx, serviceID, spec, containerName); err != nil {
			return resp, err
		}
	}
	pw.Event(progress.CreatedEvent(eventID))

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

	opts := machinedocker.PullOptions{}
	// Try to retrieve the authentication token for the image from the default local Docker config file.
	if encodedAuth, err := docker.RetrieveLocalDockerRegistryAuth(image); err == nil {
		// If RegistryAuth is empty, Uncloud daemon will try to retrieve the credentials from its own Docker config.
		opts.RegistryAuth = encodedAuth
	}

	pullCh, err := cli.Docker.PullImage(ctx, image, opts)
	if err != nil {
		statusErr := status.Convert(err)
		pw.Event(progress.Event{
			ID:         eventID,
			ParentID:   parentEventID,
			Text:       "Error",
			Status:     progress.Error,
			StatusText: statusErr.Message(),
		})
		return fmt.Errorf("pull image: %w", errors.New(statusErr.Message()))
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
			statusErr := status.Convert(err)
			pw.Event(progress.Event{
				ID:         eventID,
				ParentID:   parentEventID,
				Text:       "Error",
				Status:     progress.Error,
				StatusText: statusErr.Message(),
			})
			return fmt.Errorf("pull image: %w", errors.New(statusErr.Message()))
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

// InspectContainer returns the information about the specified container within the service.
// containerNameOrID can be name, full ID, or ID prefix of the container.
func (cli *Client) InspectContainer(
	ctx context.Context, serviceNameOrID, containerNameOrID string,
) (api.MachineServiceContainer, error) {
	svc, err := cli.InspectService(ctx, serviceNameOrID)
	if err != nil {
		return api.MachineServiceContainer{}, fmt.Errorf("inspect service: %w", err)
	}

	prefixMatchCandidates := []api.MachineServiceContainer{}
	for _, c := range svc.Containers {
		if c.Container.ID == containerNameOrID ||
			c.Container.Name == containerNameOrID {
			return c, nil
		}

		if strings.HasPrefix(c.Container.ID, containerNameOrID) {
			prefixMatchCandidates = append(prefixMatchCandidates, c)
		}
	}

	if len(prefixMatchCandidates) == 1 {
		return prefixMatchCandidates[0], nil
	} else if len(prefixMatchCandidates) > 1 {
		return api.MachineServiceContainer{}, fmt.Errorf(
			"multiple containers found with ID prefix '%s'", containerNameOrID)
	}

	return api.MachineServiceContainer{}, api.ErrNotFound
}

// containerOperationContext holds the context needed to perform an operation on a container.
type containerOperationContext struct {
	ctx         context.Context
	containerID string
	eventID     string
}

// resolveContainerOperation resolves a container by name/ID and prepares the context for an operation.
func (cli *Client) resolveContainerOperation(
	ctx context.Context, serviceNameOrID, containerNameOrID string,
) (containerOperationContext, error) {
	ctr, err := cli.InspectContainer(ctx, serviceNameOrID, containerNameOrID)
	if err != nil {
		return containerOperationContext{}, err
	}

	machine, err := cli.InspectMachine(ctx, ctr.MachineID)
	if err != nil {
		return containerOperationContext{}, fmt.Errorf("inspect machine '%s': %w", ctr.MachineID, err)
	}

	return containerOperationContext{
		ctx:         cli.ProxyMachineContext(ctx, machine.Machine.Id),
		containerID: ctr.Container.ID,
		eventID:     fmt.Sprintf("Container %s on %s", ctr.Container.Name, machine.Machine.Name),
	}, nil
}

// StartContainer starts the specified container within the service.
func (cli *Client) StartContainer(ctx context.Context, serviceNameOrID, containerNameOrID string) error {
	op, err := cli.resolveContainerOperation(ctx, serviceNameOrID, containerNameOrID)
	if err != nil {
		return err
	}

	pw := progress.ContextWriter(op.ctx)
	pw.Event(progress.StartingEvent(op.eventID))
	if err = cli.Docker.StartContainer(op.ctx, op.containerID, container.StartOptions{}); err != nil {
		return err
	}
	pw.Event(progress.StartedEvent(op.eventID))

	return nil
}

// StopContainer stops the specified container within the service.
func (cli *Client) StopContainer(
	ctx context.Context, serviceNameOrID, containerNameOrID string, opts container.StopOptions,
) error {
	op, err := cli.resolveContainerOperation(ctx, serviceNameOrID, containerNameOrID)
	if err != nil {
		return err
	}

	pw := progress.ContextWriter(op.ctx)
	pw.Event(progress.StoppingEvent(op.eventID))
	if err = cli.Docker.StopContainer(op.ctx, op.containerID, opts); err != nil {
		return err
	}
	pw.Event(progress.StoppedEvent(op.eventID))

	return nil
}

// RemoveContainer removes the specified container within the service.
func (cli *Client) RemoveContainer(
	ctx context.Context, serviceNameOrID, containerNameOrID string, opts container.RemoveOptions,
) error {
	op, err := cli.resolveContainerOperation(ctx, serviceNameOrID, containerNameOrID)
	if err != nil {
		return err
	}

	pw := progress.ContextWriter(op.ctx)
	pw.Event(progress.RemovingEvent(op.eventID))
	if err = cli.Docker.RemoveServiceContainer(op.ctx, op.containerID, opts); err != nil {
		return err
	}
	pw.Event(progress.RemovedEvent(op.eventID))

	return nil
}

// ExecContainer executes a command in a container within the service.
// If containerNameOrID is empty, the first container in the service will be used.
func (cli *Client) ExecContainer(
	ctx context.Context, serviceNameOrID, containerNameOrID string, execOpts api.ExecOptions,
) (int, error) {
	var ctr api.MachineServiceContainer

	if containerNameOrID == "" {
		// Find the first (random) container in the service
		service, err := cli.InspectService(ctx, serviceNameOrID)
		if err != nil {
			return -1, fmt.Errorf("inspect service: %w", err)
		}
		if len(service.Containers) == 0 {
			return -1, fmt.Errorf("no containers found in service %s", serviceNameOrID)
		}
		ctr = service.Containers[0]
	} else {
		// Find the specific container
		var err error
		ctr, err = cli.InspectContainer(ctx, serviceNameOrID, containerNameOrID)
		if err != nil {
			return -1, fmt.Errorf("inspect container: %w", err)
		}
	}

	machine, err := cli.InspectMachine(ctx, ctr.MachineID)
	if err != nil {
		return -1, fmt.Errorf("inspect machine '%s': %w", ctr.MachineID, err)
	}

	// Proxy Docker gRPC requests to the machine hosting the container
	ctx = cli.ProxyMachineContext(ctx, machine.Machine.Id)

	// Execute the command in the container
	exitCode, err := cli.Docker.ExecContainer(ctx, machinedocker.ExecConfig{
		ContainerID: ctr.Container.ID,
		Options:     execOpts,
	})
	if err != nil {
		return exitCode, fmt.Errorf("exec in container %s: %w", ctr.Container.Name, err)
	}

	return exitCode, nil
}

// WaitContainerHealthy polls the container until it is considered running and healthy.
//
// For containers without a health check, it waits for the monitor period and then verifies the container
// is still running and not restarting.
//
// For containers with a health check, it waits until Docker reports healthy or unhealthy. During the monitor period,
// unhealthy status is treated as retryable (the container may be recovering from a transient crash).
// After the monitor period, unhealthy becomes a permanent failure.
func (cli *Client) WaitContainerHealthy(
	ctx context.Context, serviceNameOrID, containerNameOrID string, opts api.WaitContainerHealthyOptions,
) error {
	// First inspect to get container info, machine name, and health check config.
	mc, err := cli.InspectContainer(ctx, serviceNameOrID, containerNameOrID)
	if err != nil {
		return fmt.Errorf("inspect container: %w", err)
	}

	machine, err := cli.InspectMachine(ctx, mc.MachineID)
	if err != nil {
		return fmt.Errorf("inspect machine '%s': %w", mc.MachineID, err)
	}

	pw := progress.ContextWriter(ctx)
	eventID := fmt.Sprintf("Container %s on %s", mc.Container.Name, machine.Machine.Name)

	var monitor time.Duration
	if opts.MonitorPeriod == nil {
		monitor = api.DefaultHealthMonitorPeriod
	} else {
		monitor = *opts.MonitorPeriod
	}
	pw.Event(progress.NewEvent(eventID, progress.Working, fmt.Sprintf("Monitoring (%s)", monitor)))

	// For containers without a health check, just wait for the monitor period and then check the container
	// is still running and not restarting.
	if !mc.Container.HasHealthcheck() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(monitor):
		}

		mc, err := cli.InspectContainer(ctx, serviceNameOrID, containerNameOrID)
		if err != nil {
			return fmt.Errorf("inspect container: %w", err)
		}

		if mc.Container.Healthy() {
			pw.Event(progress.RunningEvent(eventID))
			return nil
		}

		humanState, _ := mc.Container.HumanState()
		pw.Event(progress.ErrorMessageEvent(eventID, fmt.Sprintf("Unhealthy (%s)", humanState)))

		if mc.Container.State.Restarting {
			return fmt.Errorf("container is restarting after monitor period (%s): exit_code=%d",
				monitor, mc.Container.State.ExitCode)
		}
		return fmt.Errorf("container is unhealthy after monitor period (%s): %s", monitor, humanState)
	}

	// For containers with a health check, wait until Docker reports healthy or unhealthy.
	mctx := proxyToMachine(ctx, machine.Machine)
	mctx, cancel := context.WithTimeout(mctx, healthcheckTimeout(mc.Container.Config.Healthcheck))
	defer cancel()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	monitorDeadline := time.Now().Add(monitor)

	for {
		select {
		case <-mctx.Done():
			return mctx.Err()
		case <-ticker.C:
			ctr, err := cli.Docker.InspectServiceContainer(mctx, mc.Container.ID)
			if err != nil {
				pw.Event(progress.NewEvent(eventID, progress.Working,
					fmt.Sprintf("Health checking (failed to inspect container: %v)", err)))
				continue
			}

			// Reset the event status if previous inspect failed.
			eventStatus := fmt.Sprintf("Monitoring (%s)", monitor)
			if time.Now().After(monitorDeadline) {
				// TODO: provide more details about running checks or waiting so the user can see what's going on.
				eventStatus = "Health checking"
			}
			pw.Event(progress.NewEvent(eventID, progress.Working, eventStatus))

			if ctr.Healthy() {
				pw.Event(progress.Healthy(eventID))
				return nil
			}
			if time.Now().Before(monitorDeadline) {
				continue
			}

			if ctr.State.Health.Status == container.Unhealthy {
				humanState, _ := ctr.HumanState()
				pw.Event(progress.ErrorMessageEvent(eventID, fmt.Sprintf("Unhealthy (%s)", humanState)))

				if ctr.State.Restarting {
					return fmt.Errorf("container is restarting after monitor period (%s): exit_code=%d",
						monitor, ctr.State.ExitCode)
				}
				return fmt.Errorf("container is unhealthy after monitor period (%s): %s", monitor, humanState)
			}
		}
	}
}

const (
	// defaultDockerHealthcheckInterval is the default Docker interval between health check runs.
	defaultDockerHealthcheckInterval = 30 * time.Second
	// defaultDockerHealthcheckTimeout is the default Docker timeout for each health check run.
	defaultDockerHealthcheckTimeout = 30 * time.Second
	// defaultDockerHealthcheckRetries is the default Docker number of consecutive failures needed
	// to consider the container unhealthy.
	defaultDockerHealthcheckRetries = 3
)

// healthcheckTimeout computes the maximum time to wait for a container to become healthy based on
// its health check config. This is the worst case timeout to stop polling in case something goes wrong and Docker
// doesn't report the container as unhealthy after it should.
func healthcheckTimeout(hc *container.HealthConfig) time.Duration {
	if hc == nil {
		return 0
	}

	interval := hc.Interval
	if interval <= 0 {
		interval = defaultDockerHealthcheckInterval
	}
	timeout := hc.Timeout
	if timeout <= 0 {
		timeout = defaultDockerHealthcheckTimeout
	}
	retries := hc.Retries
	if retries <= 0 {
		retries = defaultDockerHealthcheckRetries
	}

	// 5s is a buffer to account for scheduling delays.
	return hc.StartPeriod + time.Duration(retries)*(interval+timeout) + 5*time.Second
}
