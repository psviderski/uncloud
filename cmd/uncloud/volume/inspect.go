package volume

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type inspectOptions struct {
	machine string
}

func NewInspectCommand() *cobra.Command {
	opts := inspectOptions{}

	cmd := &cobra.Command{
		Use:   "inspect VOLUME_NAME",
		Short: "Display detailed information on a volume.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return inspect(cmd.Context(), uncli, args[0], opts)
		},
	}

	cmd.Flags().StringVarP(&opts.machine, "machine", "m", "",
		"Name or ID of the machine where the volume is located. "+
			"If not specified, the volume will be searched across all machines.")

	return cmd
}

func inspect(ctx context.Context, uncli *cli.CLI, name string, opts inspectOptions) error {
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
		if opts.machine != "" {
			return fmt.Errorf("volume '%s' not found on machine '%s'", name, opts.machine)
		}
		return fmt.Errorf("volume '%s' not found on any machine", name)
	}
	if len(volumes) > 1 {
		fmt.Printf("Volume '%s' found on multiple machines:\n", name)
		for _, v := range volumes {
			fmt.Printf(" • %s\n", v.MachineName)
		}
		return errors.New("specify --machine flag to choose which machine to use")
	}

	data, err := json.MarshalIndent(volumes[0], "", "  ")
	if err != nil {
		return fmt.Errorf("marshal volume: %w", err)
	}
	fmt.Println(string(data))

	return nil
}
