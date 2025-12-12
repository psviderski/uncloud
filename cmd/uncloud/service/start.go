//nolint:dupl
package service

import (
	"context"
	"fmt"

	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

type startOptions struct {
	services []string
}

func NewStartCommand(groupID string) *cobra.Command {
	opts := startOptions{}
	cmd := &cobra.Command{
		Use:   "start SERVICE [SERVICE...]",
		Short: "Start one or more services.",
		Long: `Start one or more previously stopped services.

Starts all containers of the specified service(s) across all machines in the cluster.
Services can be specified by name or ID.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			opts.services = args
			return start(cmd.Context(), uncli, opts)
		},
		GroupID: groupID,
	}
	return cmd
}

func start(ctx context.Context, uncli *cli.CLI, opts startOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	for _, s := range opts.services {
		err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
			if err = client.StartService(ctx, s); err != nil {
				return fmt.Errorf("start service '%s': %w", s, err)
			}
			return nil
		}, uncli.ProgressOut(), "Starting service "+s)
	}

	return err
}
