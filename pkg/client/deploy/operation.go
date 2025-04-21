package deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/pkg/api"
)

// Operation represents a single atomic operation in a deployment process.
// Operations can be composed to form complex deployment strategies.
type Operation interface {
	// Execute performs the operation using the provided client.
	// TODO: Encapsulate the client in the operation as otherwise it gives an impression that different clients
	//  can be provided. But in reality, the operation is tightly coupled with the client that was used to create it.
	Execute(ctx context.Context, cli Client) error
	// Format returns a human-readable representation of the operation.
	Format(resolver NameResolver) string
	String() string
}

// NameResolver resolves machine and container IDs to their names.
type NameResolver interface {
	MachineName(machineID string) string
	ContainerName(containerID string) string
}

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
	return fmt.Sprintf("RunContainerOperation[service_id=%s, image=%s, machine_id=%s]",
		o.ServiceID, o.Spec.Container.Image, o.MachineID)
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
	return fmt.Sprintf("%s: Stop container [name=%s]", machineName, resolver.ContainerName(o.ContainerID))
}

func (o *StopContainerOperation) String() string {
	return fmt.Sprintf("StopContainerOperation[service_id=%s, container_id=%s, machine_id=%s]",
		o.ServiceID, o.ContainerID, o.MachineID)
}

// RemoveContainerOperation stops and removes a container from a specific machine.
type RemoveContainerOperation struct {
	ServiceID   string
	ContainerID string
	MachineID   string
}

func (o *RemoveContainerOperation) Execute(ctx context.Context, cli Client) error {
	if err := cli.StopContainer(ctx, o.ServiceID, o.ContainerID, container.StopOptions{}); err != nil {
		return fmt.Errorf("stop container: %w", err)
	}
	if err := cli.RemoveContainer(ctx, o.ServiceID, o.ContainerID, container.RemoveOptions{}); err != nil {
		return fmt.Errorf("remove container: %w", err)
	}

	return nil
}

func (o *RemoveContainerOperation) Format(resolver NameResolver) string {
	machineName := resolver.MachineName(o.MachineID)
	return fmt.Sprintf("%s: Remove container [name=%s]", machineName, resolver.ContainerName(o.ContainerID))
}

func (o *RemoveContainerOperation) String() string {
	return fmt.Sprintf("RemoveContainerOperation[service_id=%s, container_id=%s, machine_id=%s]",
		o.ServiceID, o.ContainerID, o.MachineID)
}

// CreateVolumeOperation creates a volume on a specific machine.
type CreateVolumeOperation struct {
	VolumeSpec api.VolumeSpec
	MachineID  string
}

func (o *CreateVolumeOperation) Execute(ctx context.Context, cli Client) error {
	if o.VolumeSpec.Type != api.VolumeTypeVolume {
		return fmt.Errorf("invalid volume type: '%s', expected '%s'", o.VolumeSpec.Type, api.VolumeTypeVolume)
	}

	opts := volume.CreateOptions{
		Name: o.VolumeSpec.DockerVolumeName(),
	}
	if o.VolumeSpec.VolumeOptions != nil {
		if o.VolumeSpec.VolumeOptions.Driver != nil {
			opts.Driver = o.VolumeSpec.VolumeOptions.Driver.Name
			opts.DriverOpts = o.VolumeSpec.VolumeOptions.Driver.Options
		}
		opts.Labels = o.VolumeSpec.VolumeOptions.Labels
	}

	if _, err := cli.CreateVolume(ctx, o.MachineID, opts); err != nil {
		return fmt.Errorf("create volume: %w", err)
	}

	return nil
}

func (o *CreateVolumeOperation) Format(resolver NameResolver) string {
	machineName := resolver.MachineName(o.MachineID)
	return fmt.Sprintf("%s: Create volume [name=%s]", machineName, o.VolumeSpec.DockerVolumeName())
}

func (o *CreateVolumeOperation) String() string {
	return fmt.Sprintf("CreateVolumeOperation[volume=%s, machine_id=%s]",
		o.VolumeSpec.DockerVolumeName(), o.MachineID)
}

// SequenceOperation is a composite operation that executes a sequence of operations in order.
type SequenceOperation struct {
	Operations []Operation
}

func (o *SequenceOperation) Execute(ctx context.Context, cli Client) error {
	for _, op := range o.Operations {
		if err := op.Execute(ctx, cli); err != nil {
			return err
		}
	}
	return nil
}

func (o *SequenceOperation) Format(resolver NameResolver) string {
	ops := make([]string, len(o.Operations))
	for i, op := range o.Operations {
		ops[i] = "- " + op.Format(resolver)
	}

	return strings.Join(ops, "\n")
}

func (o *SequenceOperation) String() string {
	ops := make([]string, len(o.Operations))
	for i, op := range o.Operations {
		ops[i] = op.String()
	}

	return fmt.Sprintf("SequenceOperation[%s]", strings.Join(ops, ", "))
}
