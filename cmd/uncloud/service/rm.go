package service

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"uncloud/internal/cli"
)

type rmOptions struct {
	services []string
	cluster  string
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
		&opts.cluster, "cluster", "c", "",
		"Name of the cluster. (default is the current cluster)",
	)
	return cmd
}

func rm(ctx context.Context, uncli *cli.CLI, opts rmOptions) error {
	client, err := uncli.ConnectCluster(ctx, opts.cluster)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	for _, s := range opts.services {
		if err = client.RemoveService(ctx, s); err != nil {
			return fmt.Errorf("remove service %q: %w", s, err)
		}
		fmt.Printf("Service %q removed.\n", s)
	}

	return nil
}
