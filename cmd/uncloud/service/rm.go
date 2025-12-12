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
}

func NewRmCommand(groupID string) *cobra.Command {
	opts := rmOptions{}
	cmd := &cobra.Command{
		Use:     "rm SERVICE [SERVICE...]",
		Aliases: []string{"remove", "delete"},
		Short:   "Remove one or more services.",
		Long: `Remove one or more services.

The volumes used by the services are preserved and should be removed separately
with 'uc volume rm'. Anonymous Docker volumes (automatically created from VOLUME
directives in image Dockerfiles) are automatically removed with their containers.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			opts.services = args
			return rm(cmd.Context(), uncli, opts)
		},
		GroupID: groupID,
	}
	return cmd
}

func rm(ctx context.Context, uncli *cli.CLI, opts rmOptions) error {
	client, err := uncli.ConnectCluster(ctx)
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

	return err
}
