package operation

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/pkg/api"
)

// RunContainerOperation creates and starts a new container on a specific machine.
type RunContainerOperation struct {
	ServiceID string
	Spec      api.ServiceSpec
	MachineID string
}

func (o *RunContainerOperation) Execute(ctx context.Context, cli Client) error {
	resp, err := cli.CreateContainer(ctx, o.ServiceID, o.Spec, o.MachineID)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	if err = cli.StartContainer(ctx, o.ServiceID, resp.ID); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	// TODO: wait for the container to become healthy

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
}

func (o *ReplaceContainerOperation) Execute(ctx context.Context, cli Client) error {
	stopFirst := o.Order == api.UpdateOrderStopFirst

	if stopFirst {
		if err := cli.StopContainer(ctx, o.ServiceID, o.OldContainer.ID, container.StopOptions{}); err != nil {
			return fmt.Errorf("stop old container: %w", err)
		}
	}

	// TODO: Rollback support - if new container fails to start, stop new, collect logs, and restart old container (#24)
	// TODO: When parallelism is added, rollback becomes more complex - need to track which containers
	//       were stopped and restore them all on failure
	resp, err := cli.CreateContainer(ctx, o.ServiceID, o.Spec, o.MachineID)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	if err = cli.StartContainer(ctx, o.ServiceID, resp.ID); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	// TODO: wait for the container to become healthy. If unhealthy, stop new container, collect logs, and start old.

	// For start-first, we need to stop before removing.
	// For stop-first, the container is already stopped.
	if !stopFirst {
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
