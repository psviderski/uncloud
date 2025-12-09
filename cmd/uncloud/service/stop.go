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
	services       []string
	signal         string
	timeoutChanged bool
	timeout        int
}

func NewStopCommand() *cobra.Command {
	opts := stopOptions{}
	cmd := &cobra.Command{
		Use:   "stop SERVICE [SERVICE...]",
		Short: "Stop one or more services.",
		Long:  "Stop one or more services.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			opts.services = args
			opts.timeoutChanged = cmd.Flags().Changed("timeout")
			return stop(cmd.Context(), uncli, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.signal, "signal", "s", "", "Signal to send to the container")
	cmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 0, "Seconds to wait before killing the container")
	return cmd
}

func stop(ctx context.Context, uncli *cli.CLI, opts stopOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	stopOpts := container.StopOptions{
		Signal: opts.signal,
	}
	if opts.timeoutChanged {
		stopOpts.Timeout = &opts.timeout
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
