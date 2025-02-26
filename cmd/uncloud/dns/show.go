package dns

import (
	"context"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"uncloud/internal/cli"
	"uncloud/internal/cli/client"
)

type showOptions struct {
	cluster string
}

func NewShowCommand() *cobra.Command {
	opts := showOptions{}

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print the cluster domain name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return show(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringVarP(
		&opts.cluster, "cluster", "c", "",
		"Name of the cluster. (default is the current cluster)",
	)

	return cmd
}

func show(ctx context.Context, uncli *cli.CLI, opts showOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx, opts.cluster)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	domain, err := clusterClient.GetDomain(ctx)
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			return errors.New("no domain reserved")
		}
		return err
	}

	fmt.Println(domain)
	return nil
}
