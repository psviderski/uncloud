package volume

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/docker"
	machinedocker "github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
)

type exportOptions struct {
	machine string
}

const (
	MountPoint = "/mnt/data"
	Image      = "debian:bookworm-slim"
	ExportName = "uncloud-volume-export-"
	ImportName = "uncloud-volume-import-"
)

func NewExportCommand() *cobra.Command {
	opts := exportOptions{}

	cmd := &cobra.Command{
		Use:   "export VOLUME_NAME",
		Args:  cobra.ExactArgs(1),
		Short: "Export a volume as a tar archive to standard output.",
		Long: `Export a volume as a (compressed) tar archive to standard output.

The tar archive is created using GNU tar and outputs a gzipped archive to standard output.
A file can be created by redirecting the output to a file.

	uc volume export VOLUME_NAME > volume.tar.gz
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return runExport(cmd.Context(), uncli, args[0], opts)
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

	completion.MachinesFlag(cmd)

	return cmd
}

func runExport(ctx context.Context, uncli *cli.CLI, name string, opts exportOptions) error {
	if isTTY := term.IsTerminal(os.Stdout.Fd()); isTTY {
		return fmt.Errorf("refusing to write archive contents to a terminal, redirect standard output to a file")
	}
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	volumes, err := listVolumes(ctx, client, name, opts.machine)
	if err != nil {
		return err
	}

	if len(volumes) == 0 {
		fmt.Println("No volumes found.")
		return nil
	}
	if len(volumes) != 1 {
		fmt.Println("Multiple volumes found, use --machine to specify a machine.")
		return nil
	}

	ctx = client.ProxySingleMachineContext(ctx, volumes[0].MachineID)
	config, hostConfig := containerConfig(volumes[0])
	resp, err := createContainerWithImagePull(ctx, client, ExportName, config, hostConfig)
	if err != nil {
		return err
	}

	if err := client.Docker.StartContainer(ctx, resp.ID, container.StartOptions{}); err != nil {
		return err
	}
	exitCode, err := client.Docker.ExecContainer(ctx, machinedocker.ExecConfig{
		ContainerID: resp.ID,
		Options: api.ExecOptions{
			Command:      []string{"sh", "-c", "tar cz .; touch /tmp/done"},
			AttachStdout: true,
			WorkingDir:   MountPoint,
			Stdout:       os.Stdout,
		},
	})

	if exitCode == 0 {
		return nil
	}

	return fmt.Errorf("command returned with exit code: %d", exitCode)
}

func listVolumes(ctx context.Context, client *client.Client, name, machine string) ([]api.MachineVolume, error) {
	filter := &api.VolumeFilter{
		Names: []string{name},
	}
	if machine != "" {
		filter.Machines = []string{machine}
	}

	volumes, err := client.ListVolumes(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}
	return volumes, nil
}

func createContainerWithImagePull(ctx context.Context, client *client.Client, name string, config *container.Config, hostConfig *container.HostConfig) (container.CreateResponse, error) {
	resp := container.CreateResponse{}
	suffix, err := secret.RandomAlphaNumeric(4)
	if err != nil {
		return resp, fmt.Errorf("generate random suffix: %w", err)
	}
	containerName := name + suffix

	if resp, err = client.Docker.CreateContainer(ctx, config, hostConfig, nil, nil, containerName); err == nil {
		return resp, nil
	}

	if !errdefs.IsNotFound(err) {
		return resp, fmt.Errorf("create Docker container: %w", err)
	}

	if err := pull(ctx, client, config); err != nil {
		return resp, fmt.Errorf("pull Docker container: %s", err)
	}

	// Create container again after image pull.
	if resp, err = client.Docker.CreateContainer(ctx, config, hostConfig, nil, nil, containerName); err != nil {
		return resp, fmt.Errorf("create Docker container: %w", err)
	}

	return resp, nil
}

func pull(ctx context.Context, client *client.Client, config *container.Config) error {
	opts := machinedocker.PullOptions{}
	if encodedAuth, err := docker.RetrieveLocalDockerRegistryAuth(config.Image); err == nil {
		opts.RegistryAuth = encodedAuth
	}

	pullCh, err := client.Docker.PullImage(ctx, config.Image, opts)
	if err != nil {
		return fmt.Errorf("pull image: %w", err)
	}

	for msg := range pullCh {
		if msg.Err != nil {
			return fmt.Errorf("pull image: %w", err)
		}
	}
	return nil
}

func containerConfig(volume api.MachineVolume) (*container.Config, *container.HostConfig) {
	config := &container.Config{
		Image: Image,
		Labels: map[string]string{
			api.LabelManaged: "",
		},
		// The entry point waits until a /tmp/done exists and then quits, both export and import write this
		// file, and the container should then be auto-removed.
		Entrypoint:   []string{"sh", "-c", "while [ ! -e /tmp/done ]; do sleep 0.2; done"},
		WorkingDir:   volume.Volume.Mountpoint,
		AttachStdout: true,
	}
	hostConfig := &container.HostConfig{
		AutoRemove: true,
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyDisabled,
		},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: volume.Volume.Mountpoint,
				Target: MountPoint,
			},
		},
	}
	return config, hostConfig
}
