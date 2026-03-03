package operation

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/pkg/api"
)

// CreateVolumeOperation creates a volume on a specific machine.
type CreateVolumeOperation struct {
	VolumeSpec api.VolumeSpec
	MachineID  string
	// MachineName is used for formatting the operation output only.
	MachineName string
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

func (o *CreateVolumeOperation) Format(_ NameResolver) string {
	return fmt.Sprintf("%s: Create volume [name=%s]", o.MachineName, o.VolumeSpec.DockerVolumeName())
}

func (o *CreateVolumeOperation) String() string {
	return fmt.Sprintf("CreateVolumeOperation[machine_id=%s volume=%s]",
		o.MachineID, o.VolumeSpec.DockerVolumeName())
}
