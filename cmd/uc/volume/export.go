package volume

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/docker/cli/cli/streams"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
)

type exportOptions struct {
	machine string
	quiet   bool
}

const MountPoint = "/mnt/data"

func NewExportCommand() *cobra.Command {
	opts := exportOptions{}

	cmd := &cobra.Command{
		Use:     "export VOLUME_NAME [FILE]",
		Aliases: []string{"list"},
		Args:    cobra.ExactArgs(1),
		Short:   "Export a volume as a tar archive to standard output.",
		Long: `Export a volume a (gzipped) tar archive to standard output.

TODO
TODO`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return export(cmd.Context(), uncli, args[0], opts)
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return completion.Volumes(cmd.Context(), uncli, args, toComplete)
		},
	}

	cmd.Flags().StringVarP(&opts.machine, "machine", "m", "",
		"Name or ID of the machine where the volume is located. "+
			"If not specified, the volume will be searched across all machines.")
	cmd.Flags().BoolVarP(&opts.quiet, "quiet", "q", false,
		"Hide all output.")

	completion.MachinesFlag(cmd)

	return cmd
}

func export(ctx context.Context, uncli *cli.CLI, name string, opts exportOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	filter := &api.VolumeFilter{
		Names: []string{name},
	}
	if opts.machine != "" {
		filter.Machines = []string{opts.machine}
	}

	volumes, err := client.ListVolumes(ctx, filter)
	if err != nil {
		return fmt.Errorf("list volumes: %w", err)
	}

	if len(volumes) == 0 {
		fmt.Println("No volumes found.")
		return nil
	}
	if len(volumes) != 1 {
		fmt.Println("Multiple volumes found, use --machine to specify a machine.")
		return nil
	}

	spec, err := prepareServiceSpec(volumes[0], "export")
	if err != nil {
		return err
	}

	var resp api.RunServiceResponse
	if !opts.quiet {
		err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
			resp, err = client.RunService(ctx, spec)
			if err != nil {
				return fmt.Errorf("run service: %w", err)
			}
			return nil
		}, streams.NewOut(os.Stderr), fmt.Sprintf("Running service %s", spec.Name))
	} else {
		resp, err = client.RunService(ctx, spec)
	}
	if err != nil {
		return err
	}

	defer func() {
		if !opts.quiet {
			err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
				if err = client.RemoveService(ctx, spec.Name); err != nil {
					return fmt.Errorf("remove service '%s': %w", spec.Name, err)
				}
				return nil
			}, streams.NewOut(os.Stderr), "Removing service "+spec.Name)
		} else {
			client.RemoveService(ctx, spec.Name)
		}
	}()
	defer client.StopService(ctx, resp.ID, container.StopOptions{Timeout: new(1)})

	return runCommand(ctx, client, resp.ID, []string{"tar", "-cz", "."})
}

func runCommand(ctx context.Context, client *client.Client, serviceID string, cmd []string) error {
	svc, err := client.InspectService(ctx, serviceID)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			return fmt.Errorf("service '%s' not found in the cluster", serviceID)
		}
		return fmt.Errorf("inspect service '%s': %w", serviceID, err)
	}

	var ctr *api.MachineServiceContainer
	for i := range svc.Containers {
		if svc.Containers[i].Container.Healthy() {
			ctr = &svc.Containers[i]
			break
		}
	}
	if ctr == nil {
		return fmt.Errorf("no running healthy container found for service '%s'", serviceID)
	}

	exitCode, err := client.ExecContainer(ctx, serviceID, ctr.Container.ID, api.ExecOptions{
		Command:      cmd,
		WorkingDir:   MountPoint,
		AttachStdout: true,
		// Not attaching Stderr.
		Stdout: os.Stdout,
	})

	if exitCode == 0 {
		return err // should be nil also
	}

	return fmt.Errorf("command returned with exit code: %d: %w", exitCode, err)
}

func prepareServiceSpec(volume api.MachineVolume, operation string) (api.ServiceSpec, error) {
	var spec api.ServiceSpec
	suffix, err := secret.RandomAlphaNumeric(4)
	if err != nil {
		return spec, fmt.Errorf("generate random suffix: %w", err)
	}
	spec = api.ServiceSpec{
		Name: "uncloud-volume-" + operation,
		Mode: api.ServiceModeReplicated,
		Container: api.ContainerSpec{
			Image:           "busybox:latest",
			Entrypoint:      []string{"sleep"},
			Command:         []string{"infinity"},
			PullPolicy:      api.PullPolicyMissing,
			StopGracePeriod: new(time.Second),
			VolumeMounts: []api.VolumeMount{
				{
					VolumeName:    "bind-" + suffix,
					ContainerPath: MountPoint,
					ReadOnly:      true,
				},
			},
		},
		UpdateConfig: api.UpdateConfig{
			MonitorPeriod: new(time.Second),
		},
		Replicas: 1,
		Placement: api.Placement{
			Machines: []string{volume.MachineName},
		},
		Volumes: []api.VolumeSpec{
			{
				Name:        "bind-" + suffix,
				Type:        api.VolumeTypeBind,
				BindOptions: &api.BindOptions{HostPath: volume.Volume.Mountpoint},
			},
		},
	}

	return spec, nil
}
