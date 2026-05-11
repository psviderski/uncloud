package operation

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stringid"
	cliprogress "github.com/psviderski/uncloud/internal/cli/progress"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
)

// ContainerHealthError indicates that a service container failed to become healthy during deployment.
type ContainerHealthError struct {
	error
	ServiceName string
	ContainerID string
	MachineName string
}

func (e *ContainerHealthError) Unwrap() error {
	return e.error
}

// RunContainerOperation creates and starts a new container on a specific machine.
type RunContainerOperation struct {
	ServiceID string
	Spec      api.ServiceSpec
	MachineID string
	// MachineName is used for formatting the operation as part of the deployment plan.
	MachineName string
	// SkipHealthMonitor skips the monitoring period and health checks after starting a container.
	SkipHealthMonitor bool
}

func (o *RunContainerOperation) Execute(ctx context.Context, cli Client) error {
	resp, err := cli.CreateContainer(ctx, o.ServiceID, o.Spec, o.MachineID)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	// Override event ID so StartContainer and WaitContainerHealthy update the same progress line as creation.
	// TODO: This is a hack to work around the limitations of the compose progress library.
	//  We likely need to fork or create our own to decouple event IDs from presentation layer.
	ctx = cliprogress.WithEventID(ctx, cliprogress.NewContainerEventID(ctx, resp.Name, o.MachineName))
	if err = cli.StartContainer(ctx, o.ServiceID, resp.ID); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	if o.SkipHealthMonitor {
		return nil
	}

	opts := api.WaitContainerHealthyOptions{MonitorPeriod: o.Spec.UpdateConfig.MonitorPeriod}
	if err = cli.WaitContainerHealthy(ctx, o.ServiceID, resp.ID, opts); err != nil {
		return &ContainerHealthError{
			error: fmt.Errorf("container '%s/%s' failed to become healthy: %w",
				o.Spec.Name, stringid.TruncateID(resp.ID), err),
			ServiceName: o.Spec.Name,
			ContainerID: resp.ID,
			MachineName: o.MachineName,
		}
	}

	return nil
}

func (o *RunContainerOperation) Format() string {
	return tui.BoldGreen.Render("+") + "   " +
		tui.Faint.Render("run container") + " " +
		o.Spec.Name + " " +
		tui.Faint.Render("on") + " " +
		o.MachineName
}

func (o *RunContainerOperation) String() string {
	return fmt.Sprintf("RunContainerOperation[machine_id=%s service_id=%s image=%s]",
		o.MachineID, o.ServiceID, o.Spec.Container.Image)
}

// StopContainerOperation stops a container on a specific machine.
type StopContainerOperation struct {
	ServiceID   string
	ContainerID string
	MachineID   string
	// MachineName is used for formatting the operation as part of the deployment plan.
	MachineName     string
	StopGracePeriod *time.Duration
}

func (o *StopContainerOperation) Execute(ctx context.Context, cli Client) error {
	if err := cli.StopContainer(ctx, o.ServiceID, o.ContainerID, stopOptions(o.StopGracePeriod)); err != nil {
		return fmt.Errorf("stop container: %w", err)
	}
	return nil
}

func (o *StopContainerOperation) Format() string {
	// TODO: pass service name to format the display name consistently with other operations.
	displayName := stringid.TruncateID(o.ContainerID)

	return tui.BoldRed.Render("-") + "   " +
		tui.Faint.Render("stop container") + " " +
		displayName + " " +
		tui.Faint.Render("on") + " " +
		o.MachineName
}

func (o *StopContainerOperation) String() string {
	return fmt.Sprintf("StopContainerOperation[machine_id=%s service_id=%s container_id=%s]",
		o.MachineID, o.ServiceID, o.ContainerID)
}

// RemoveContainerOperation stops and removes a container from a specific machine.
type RemoveContainerOperation struct {
	MachineID string
	// MachineName is used for formatting the operation as part of the deployment plan.
	MachineName     string
	Container       api.ServiceContainer
	StopGracePeriod *time.Duration
}

func (o *RemoveContainerOperation) Execute(ctx context.Context, cli Client) error {
	err := cli.StopContainer(ctx, o.Container.ServiceID(), o.Container.ID, stopOptions(o.StopGracePeriod))
	if err != nil {
		return fmt.Errorf("stop container: %w", err)
	}

	if err = cli.RemoveContainer(ctx, o.Container.ServiceID(), o.Container.ID, container.RemoveOptions{
		// Remove anonymous volumes created by the container.
		RemoveVolumes: true,
	}); err != nil {
		return fmt.Errorf("remove container: %w", err)
	}

	return nil
}

func (o *RemoveContainerOperation) Format() string {
	displayName := o.Container.ServiceSpec.Name + tui.Faint.Render("/") + o.Container.ShortID()

	return tui.BoldRed.Render("-") + "   " +
		tui.Faint.Render("remove container") + " " +
		displayName + " " +
		tui.Faint.Render("on") + " " +
		o.MachineName
}

func (o *RemoveContainerOperation) String() string {
	return fmt.Sprintf("RemoveContainerOperation[machine_id=%s service_id=%s container_id=%s]",
		o.MachineID, o.Container.ServiceID(), o.Container.ID)
}

// ReplaceContainerOperation replaces an old container with a new one based on the specified update order.
// For start-first: starts new container, then removes old container.
// For stop-first: stops old container, starts new container, then removes old container.
type ReplaceContainerOperation struct {
	ServiceID string
	Spec      api.ServiceSpec
	MachineID string
	// MachineName is used for formatting the operation as part of the deployment plan.
	MachineName  string
	OldContainer api.ServiceContainer
	// Order specifies the update order: "start-first" or "stop-first".
	Order string
	// SkipHealthMonitor skips the monitoring period and health checks after starting a new container.
	SkipHealthMonitor bool
	StopGracePeriod   *time.Duration
}

func (o *ReplaceContainerOperation) Execute(ctx context.Context, cli Client) error {
	stopFirst := o.Order == api.UpdateOrderStopFirst

	wasRunning := false
	if stopFirst {
		// Inspect the old container to remember its running state before stopping.
		ctr, err := cli.InspectContainer(ctx, o.ServiceID, o.OldContainer.ID)
		if err != nil {
			return fmt.Errorf("inspect old container: %w", err)
		}
		wasRunning = ctr.Container.State.Running
		if wasRunning {
			err = cli.StopContainer(ctx, o.ServiceID, o.OldContainer.ID, stopOptions(o.StopGracePeriod))
			if err != nil {
				return fmt.Errorf("stop old container: %w", err)
			}
		}
	}

	resp, err := cli.CreateContainer(ctx, o.ServiceID, o.Spec, o.MachineID)
	if err != nil {
		return fmt.Errorf("create new container: %w", err)
	}
	// Override event ID so StartContainer and WaitContainerHealthy update the same progress line as creation.
	newCtx := cliprogress.WithEventID(ctx, cliprogress.NewContainerEventID(ctx, resp.Name, o.MachineName))
	if err = cli.StartContainer(newCtx, o.ServiceID, resp.ID); err != nil {
		return fmt.Errorf("start new container: %w", err)
	}

	if !o.SkipHealthMonitor {
		opts := api.WaitContainerHealthyOptions{MonitorPeriod: o.Spec.UpdateConfig.MonitorPeriod}
		if err = cli.WaitContainerHealthy(newCtx, o.ServiceID, resp.ID, opts); err != nil {
			// New container failed to become healthy. Stop it and roll back to the previous container.
			// Don't remove the new stopped container to allow users to inspect logs and state.

			// Use context without progress to not overwrite the container Unhealthy status with Stopped.
			ctxWithoutProgress := progress.WithContextWriter(ctx, nil)
			_ = cli.StopContainer(ctxWithoutProgress, o.ServiceID, resp.ID, stopOptions(o.StopGracePeriod))

			newCtr := fmt.Sprintf("%s/%s", o.Spec.Name, stringid.TruncateID(resp.ID))
			healthErr := fmt.Errorf(
				"new container '%s' failed to become healthy: %w. "+
					"It's stopped and available for inspection. View logs with 'uc logs %s'",
				newCtr, err, newCtr,
			)

			finalErr := healthErr
			if stopFirst && wasRunning {
				// Restart the old container only if it was running before we stopped it.
				oldCtr := fmt.Sprintf("%s/%s", o.OldContainer.ServiceSpec.Name, o.OldContainer.ShortID())
				if rollbackErr := cli.StartContainer(ctx, o.ServiceID, o.OldContainer.ID); rollbackErr != nil {
					finalErr = fmt.Errorf("%w. Rolled back to old container '%s' but failed to restart it: %w",
						healthErr, oldCtr, rollbackErr)
				} else {
					finalErr = fmt.Errorf("%w. Rolled back to old container '%s'", healthErr, oldCtr)
				}
			}

			return &ContainerHealthError{
				error:       finalErr,
				ServiceName: o.Spec.Name,
				ContainerID: resp.ID,
				MachineName: o.MachineName,
			}
		}
	}

	// For start-first, we need to stop before removing.
	// For stop-first, the container is already stopped.
	if !stopFirst {
		// TODO: the new container is propagated to Caddy upstreams through the cluster store asynchronously.
		//  There still might be a brief downtime (for a 1 replica service) when Caddy doesn't know about
		//  the new container but we're stopping the old container. We should somehow ensure Caddy is updated
		//  with the new container before we stop the old one to avoid this downtime.
		if err = cli.StopContainer(ctx, o.ServiceID, o.OldContainer.ID, stopOptions(o.StopGracePeriod)); err != nil {
			return fmt.Errorf("stop old container: %w", err)
		}
	}

	if err = cli.RemoveContainer(ctx, o.ServiceID, o.OldContainer.ID, container.RemoveOptions{
		RemoveVolumes: true,
	}); err != nil {
		return fmt.Errorf("remove old container: %w", err)
	}

	return nil
}

func (o *ReplaceContainerOperation) Format() string {
	displayName := o.Spec.Name + tui.Faint.Render("/") + o.OldContainer.ShortID()

	if o.Order == api.UpdateOrderStopFirst {
		return tui.BoldYellow.Render("-") + tui.Yellow.Render("/") + tui.BoldYellow.Render("+") + " " +
			tui.Faint.Render("replace container") + " " +
			displayName + " " +
			tui.Faint.Render("on") + " " +
			o.MachineName + " " +
			tui.Yellow.Render("(stop-first)")
	}
	return tui.BoldGreen.Render("+") + tui.Green.Render("/") + tui.BoldGreen.Render("-") + " " +
		tui.Faint.Render("replace container") + " " +
		displayName + " " +
		tui.Faint.Render("on") + " " +
		o.MachineName
}

func (o *ReplaceContainerOperation) String() string {
	return fmt.Sprintf("ReplaceContainerOperation[machine_id=%s service_id=%s old_container_id=%s order=%s]",
		o.MachineID, o.ServiceID, o.OldContainer.ID, o.Order)
}

// stopOptions converts a stop grace period duration to Docker container stop options.
func stopOptions(gracePeriod *time.Duration) container.StopOptions {
	if gracePeriod == nil {
		return container.StopOptions{}
	}
	t := int(gracePeriod.Seconds())
	return container.StopOptions{Timeout: &t}
}
