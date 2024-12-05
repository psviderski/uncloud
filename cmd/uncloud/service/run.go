package service

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"uncloud/internal/api"
	"uncloud/internal/cli"
)

type runOptions struct {
	command []string
	image   string
	machine string
	mode    string
	name    string
	publish []string

	cluster string
}

func NewRunCommand() *cobra.Command {
	opts := runOptions{}

	cmd := &cobra.Command{
		Use:   "run IMAGE [COMMAND...]",
		Short: "Run a service.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			opts.image = args[0]
			if len(args) > 1 {
				opts.command = args[1:]
			}

			return runRun(cmd.Context(), uncli, opts)
		},
	}

	// TODO: implement placement constraints and translate --machine to a constraint.
	//cmd.Flags().StringVarP(
	//	&opts.machine, "machine", "m", "",
	//	"Name or ID of the machine to run the service on. (default is first available)",
	//)
	cmd.Flags().StringVar(&opts.mode, "mode", api.ServiceModeReplicated,
		fmt.Sprintf("Replication mode of the service: either %q (a specified number of containers across "+
			"the machines) or %q (one container on every machine).",
			api.ServiceModeReplicated, api.ServiceModeGlobal))
	cmd.Flags().StringVarP(&opts.name, "name", "n", "",
		"Assign a name to the service. A random name is generated if not specified.")
	cmd.Flags().StringSliceVarP(&opts.publish, "publish", "p", nil,
		"Publish a service port to make it accessible outside the cluster. Can be specified multiple times. "+
			"Format: [load_balancer_port:]container_port[/protocol]")

	cmd.Flags().StringVarP(
		&opts.cluster, "cluster", "c", "",
		"Name of the cluster to run the service in. (default is the current cluster)",
	)

	return cmd
}

func runRun(ctx context.Context, uncli *cli.CLI, opts runOptions) error {
	switch opts.mode {
	case "", api.ServiceModeReplicated, api.ServiceModeGlobal:
	default:
		return fmt.Errorf("invalid replication mode: %q", opts.mode)
	}

	client, err := uncli.ConnectCluster(ctx, opts.cluster)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	spec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Command: opts.command,
			Image:   opts.image,
		},
		Mode: opts.mode,
		Name: opts.name,
	}
	resp, err := client.RunService(ctx, spec)
	if err != nil {
		return fmt.Errorf("run service: %w", err)
	}

	fmt.Printf("Service %q started.\n", resp.Name)
	return nil
}
