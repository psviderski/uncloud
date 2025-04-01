package dns

import (
	"context"
	"errors"
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type showOptions struct {
	context string
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
		&opts.context, "context", "c", "",
		"Name of the cluster context. (default is the current context)",
	)

	return cmd
}

func show(ctx context.Context, uncli *cli.CLI, opts showOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx, opts.context)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	domain, err := clusterClient.GetDomain(ctx)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			return errors.New("no domain reserved")
		}
		return err
	}

	fmt.Println(domain)
	return nil
}
