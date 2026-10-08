package volume

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	machinedocker "github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type importOptions struct {
	machine string
	user    string
	quiet   bool
}

func NewImportCommand() *cobra.Command {
	opts := importOptions{}

	cmd := &cobra.Command{
		Use:   "import VOLUME_NAME",
		Args:  cobra.ExactArgs(1),
		Short: "Import a volume from a tar archive from standard input.",
		Long: `Import a volume as (compressed) tar archive from standard input.

The tar archive is copied from standard input to a GNU tar running in a container.

If you have a (gzipped) tar archive you can import this to a new volume with:

	uc volume import VOLUME_NAME < volume.tar.gz

When importing the files are printed to standard output. Copying a volume on the fly can be done
by piping the output from 'uc volume export' into import:

	uc volume export OLD_VOLUME | uc volume export NEW_VOLUME
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return runImport(cmd.Context(), uncli, args[0], opts)
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

func runImport(ctx context.Context, uncli *cli.CLI, name string, opts importOptions) error {
	if isTTY := term.IsTerminal(os.Stdin.Fd()); isTTY {
		return fmt.Errorf("refusing to read archive contents from a terminal, redirect standard input from a file")
	}
	client, err := uncli.ConnectClusterWithOptions(ctx, cli.ConnectOptions{})
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	volumes, err := listVolumes(ctx, client, name, opts.machine)
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
			Command:      []string{"sh", "-c", "tar xvz; touch /tmp/done"},
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			WorkingDir:   MountPoint,
			Stdin:        os.Stdin,
			Stdout:       os.Stdout,
			Stderr:       os.Stderr,
		},
	})

	if exitCode == 0 {
		return nil
	}

	return fmt.Errorf("command returned with exit code: %d", exitCode)

}
