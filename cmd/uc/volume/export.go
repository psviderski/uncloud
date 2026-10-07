package volume

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func NewExportCommand() *cobra.Command {
	opts := exportOptions{}

	cmd := &cobra.Command{
		Use:     "export VOLUME_NAME [FILE]",
		Aliases: []string{"list"},
		Args:    cobra.ExactArgs(1),
		Short:   "Export a volume as a tar archive to standard output.",
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

	suffix, err := secret.RandomAlphaNumeric(4)
	if err != nil {
		return fmt.Errorf("generate random suffix: %w", err)
	}

	volume := volumes[0]
	spec := api.ServiceSpec{
		Name: "uncloud-volume-export",
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
					ContainerPath: "/mnt/data",
					ReadOnly:      true,
				},
			},
		},
		UpdateConfig: api.UpdateConfig{
			MonitorPeriod: new(time.Second),
		},
		Replicas: 1,
		Placement: api.Placement{
			Machines: []string{volumes[0].MachineName},
		},
		Volumes: []api.VolumeSpec{
			{
				Name:        "bind-" + suffix,
				Type:        api.VolumeTypeBind,
				BindOptions: &api.BindOptions{HostPath: filepath.Dir(volume.Volume.Mountpoint)},
			},
		},
	}
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("invalid service configuration: %w", err)
	}

	var resp api.RunServiceResponse
	if !opts.quiet {
		err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
			resp, err = client.RunService(ctx, spec)
			if err != nil {
				return fmt.Errorf("run service: %w", err)
			}
			return nil
		}, streams.NewOut(os.Stderr), fmt.Sprintf("Running service %s (%s mode)", spec.Name, spec.Mode))
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

	return runAndCapture(ctx, client, resp.ID)
}

func runAndCapture(ctx context.Context, client *client.Client, serviceID string) error {
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
		Command:      []string{"tree", "/mnt/data"},
		AttachStdout: true,
		Stdout:       os.Stdout,
	})

	if exitCode == 0 {
		return err // should be nil also
	}

	return fmt.Errorf("tar return exit code: %d: %w", exitCode, err)
}
