//nolint:dupl
package service

import (
	"context"
	"fmt"

	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

type stopOptions struct {
	services []string
	signal   string
	timeout  int
}

func NewStopCommand(groupID string) *cobra.Command {
	opts := stopOptions{}
	cmd := &cobra.Command{
		Use:   "stop SERVICE [SERVICE...]",
		Short: "Stop one or more services.",
		Long: `Stop one or more running services.

Gracefully stops all containers of the specified service(s) across all machines in the cluster.
Services can be specified by name or ID. Stopped services can be restarted with 'uc start'.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			opts.services = args
			return stop(cmd.Context(), uncli, opts)
		},
		GroupID: groupID,
	}
	cmd.Flags().StringVarP(&opts.signal, "signal", "s", "",
		"Signal to send to each container's main process.\n"+
			"Can be a signal name (SIGTERM, SIGINT, SIGHUP, etc.) or a number. (default SIGTERM)")
	cmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 10,
		"Seconds to wait for each container to stop gracefully before forcibly killing it with SIGKILL.\n"+
			"Use -1 to wait indefinitely.")
	return cmd
}

func stop(ctx context.Context, uncli *cli.CLI, opts stopOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	stopOpts := container.StopOptions{
		Signal:  opts.signal,
		Timeout: &opts.timeout,
	}

	for _, s := range opts.services {
		err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
			if err = client.StopService(ctx, s, stopOpts); err != nil {
				return fmt.Errorf("stop service '%s': %w", s, err)
			}
			return nil
		}, uncli.ProgressOut(), "Stopping service "+s)
	}

	return err
}
