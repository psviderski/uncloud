package operation

import (
	"context"
	"fmt"

	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stringid"
	"github.com/psviderski/uncloud/pkg/api"
)

// RunContainerOperation creates and starts a new container on a specific machine.
type RunContainerOperation struct {
	ServiceID string
	Spec      api.ServiceSpec
	MachineID string
	// SkipHealthMonitor skips the monitoring period and health checks after starting a container.
	SkipHealthMonitor bool
}

func (o *RunContainerOperation) Execute(ctx context.Context, cli Client) error {
	resp, err := cli.CreateContainer(ctx, o.ServiceID, o.Spec, o.MachineID)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	if err = cli.StartContainer(ctx, o.ServiceID, resp.ID); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	if o.SkipHealthMonitor {
		return nil
	}

	opts := api.WaitContainerHealthyOptions{MonitorPeriod: o.Spec.UpdateConfig.MonitorPeriod}
	if err = cli.WaitContainerHealthy(ctx, o.ServiceID, resp.ID, opts); err != nil {
		return fmt.Errorf("container '%s/%s' failed to become healthy: %w",
			o.Spec.Name, stringid.TruncateID(resp.ID), err)
	}

	return nil
}

func (o *RunContainerOperation) Format(resolver NameResolver) string {
	machineName := resolver.MachineName(o.MachineID)
	return fmt.Sprintf("%s: Run container [image=%s]", machineName, o.Spec.Container.Image)
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
}

func (o *StopContainerOperation) Execute(ctx context.Context, cli Client) error {
	if err := cli.StopContainer(ctx, o.ServiceID, o.ContainerID, container.StopOptions{}); err != nil {
		return fmt.Errorf("stop container: %w", err)
	}
	return nil
}

func (o *StopContainerOperation) Format(resolver NameResolver) string {
	machineName := resolver.MachineName(o.MachineID)
	return fmt.Sprintf("%s: Stop container [id=%s name=%s]", machineName,
		o.ContainerID[:12], resolver.ContainerName(o.ContainerID))
}

func (o *StopContainerOperation) String() string {
	return fmt.Sprintf("StopContainerOperation[machine_id=%s service_id=%s container_id=%s]",
		o.MachineID, o.ServiceID, o.ContainerID)
}

// RemoveContainerOperation stops and removes a container from a specific machine.
type RemoveContainerOperation struct {
	MachineID string
	Container api.ServiceContainer
}

func (o *RemoveContainerOperation) Execute(ctx context.Context, cli Client) error {
	if err := cli.StopContainer(ctx, o.Container.ServiceID(), o.Container.ID, container.StopOptions{}); err != nil {
		return fmt.Errorf("stop container: %w", err)
	}
	if err := cli.RemoveContainer(ctx, o.Container.ServiceID(), o.Container.ID, container.RemoveOptions{
		// Remove anonymous volumes created by the container.
		RemoveVolumes: true,
	}); err != nil {
		return fmt.Errorf("remove container: %w", err)
	}

	return nil
}

func (o *RemoveContainerOperation) Format(resolver NameResolver) string {
	machineName := resolver.MachineName(o.MachineID)
	return fmt.Sprintf("%s: Remove container [id=%s image=%s]",
		machineName, o.Container.ShortID(), o.Container.Config.Image)
}

func (o *RemoveContainerOperation) String() string {
	return fmt.Sprintf("RemoveContainerOperation[machine_id=%s service_id=%s container_id=%s]",
		o.MachineID, o.Container.ServiceID(), o.Container.ID)
}

// ReplaceContainerOperation replaces an old container with a new one based on the specified update order.
// For start-first: starts new container, then removes old container.
// For stop-first: stops old container, starts new container, then removes old container.
type ReplaceContainerOperation struct {
	ServiceID    string
	Spec         api.ServiceSpec
	MachineID    string
	OldContainer api.ServiceContainer
	// Order specifies the update order: "start-first" or "stop-first".
	Order string
	// SkipHealthMonitor skips the monitoring period and health checks after starting a new container.
	SkipHealthMonitor bool
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
			if err = cli.StopContainer(ctx, o.ServiceID, o.OldContainer.ID, container.StopOptions{}); err != nil {
				return fmt.Errorf("stop old container: %w", err)
			}
		}
	}

	resp, err := cli.CreateContainer(ctx, o.ServiceID, o.Spec, o.MachineID)
	if err != nil {
		return fmt.Errorf("create new container: %w", err)
	}
	if err = cli.StartContainer(ctx, o.ServiceID, resp.ID); err != nil {
		return fmt.Errorf("start new container: %w", err)
	}

	if !o.SkipHealthMonitor {
		opts := api.WaitContainerHealthyOptions{MonitorPeriod: o.Spec.UpdateConfig.MonitorPeriod}
		if err = cli.WaitContainerHealthy(ctx, o.ServiceID, resp.ID, opts); err != nil {
			// New container failed to become healthy. Stop it and roll back to the previous container.
			// Don't remove the new stopped container to allow users to inspect logs and state.
			// TODO: collect logs from the new container and include in the error message to speed up debugging.

			// Use context without progress to not overwrite the container Unhealthy status with Stopped.
			ctxWithoutProgress := progress.WithContextWriter(ctx, nil)
			_ = cli.StopContainer(ctxWithoutProgress, o.ServiceID, resp.ID, container.StopOptions{})

			newCtr := fmt.Sprintf("%s/%s", o.Spec.Name, stringid.TruncateID(resp.ID))
			healthErr := fmt.Errorf(
				"new container '%s' failed to become healthy: %w. "+
					"It's stopped and available for inspection. Fetch logs with 'uc logs %s'",
				newCtr, err, o.Spec.Name,
			)

			if stopFirst && wasRunning {
				// Restart the old container only if it was running before we stopped it.
				oldCtr := fmt.Sprintf("%s/%s", o.OldContainer.ServiceSpec.Name, o.OldContainer.ShortID())
				if rollbackErr := cli.StartContainer(ctx, o.ServiceID, o.OldContainer.ID); rollbackErr != nil {
					return fmt.Errorf("%w. Rolled back to old container '%s' but failed to restart it: %w",
						healthErr, oldCtr, rollbackErr)
				}
				return fmt.Errorf("%w. Rolled back to old container '%s'", healthErr, oldCtr)
			}

			return healthErr
		}
	}

	// For start-first, we need to stop before removing.
	// For stop-first, the container is already stopped.
	if !stopFirst {
		// TODO: the new container is propagated to Caddy upstreams through the cluster store asynchronously.
		//  There still might be a brief downtime (for a 1 replica service) when Caddy doesn't know about
		//  the new container but we're stopping the old container. We should somehow ensure Caddy is updated
		//  with the new container before we stop the old one to avoid this downtime.
		if err := cli.StopContainer(ctx, o.ServiceID, o.OldContainer.ID, container.StopOptions{}); err != nil {
			return fmt.Errorf("stop old container: %w", err)
		}
	}

	if err := cli.RemoveContainer(ctx, o.ServiceID, o.OldContainer.ID, container.RemoveOptions{
		RemoveVolumes: true,
	}); err != nil {
		return fmt.Errorf("remove old container: %w", err)
	}

	return nil
}

func (o *ReplaceContainerOperation) Format(resolver NameResolver) string {
	return fmt.Sprintf("%s: Replace container [id=%s image=%s order=%s]",
		resolver.MachineName(o.MachineID), o.OldContainer.ShortID(), o.Spec.Container.Image, o.Order)
}

func (o *ReplaceContainerOperation) String() string {
	return fmt.Sprintf("ReplaceContainerOperation[machine_id=%s service_id=%s old_container_id=%s order=%s]",
		o.MachineID, o.ServiceID, o.OldContainer.ID, o.Order)
}
