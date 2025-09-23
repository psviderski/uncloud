package service

import (
	"context"
	"fmt"

	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

type rmOptions struct {
	services []string
	context  string
}

func NewRmCommand() *cobra.Command {
	opts := rmOptions{}
	cmd := &cobra.Command{
		Use:     "rm SERVICE [SERVICE...]",
		Aliases: []string{"remove", "delete"},
		Short:   "Remove one or more services.",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			opts.services = args
			return rm(cmd.Context(), uncli, opts)
		},
	}
	cmd.Flags().StringVarP(
		&opts.context, "context", "c", "",
		"Name of the cluster context. (default is the current context)",
	)
	return cmd
}

func rm(ctx context.Context, uncli *cli.CLI, opts rmOptions) error {
	client, err := uncli.ConnectCluster(ctx, opts.context)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	for _, s := range opts.services {
		err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
			if err = client.RemoveService(ctx, s); err != nil {
				return fmt.Errorf("remove service '%s': %w", s, err)
			}
			return nil
		}, uncli.ProgressOut(), "Removing service "+s)
	}

	return nil
}
