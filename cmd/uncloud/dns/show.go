package dns

import (
	"context"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"uncloud/internal/cli"
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
	client, err := uncli.ConnectCluster(ctx, opts.cluster)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	domain, err := client.GetDomain(ctx, &emptypb.Empty{})
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			return errors.New("no domain reserved")
		}
		return err
	}

	fmt.Println(domain.Name)
	return nil
}
