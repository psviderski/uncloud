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

type releaseOptions struct {
	cluster string
}

func NewReleaseCommand() *cobra.Command {
	opts := releaseOptions{}

	cmd := &cobra.Command{
		Use:   "release",
		Short: "Release the reserved cluster domain.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return release(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringVarP(
		&opts.cluster, "cluster", "c", "",
		"Name of the cluster. (default is the current cluster)",
	)

	return cmd
}

func release(ctx context.Context, uncli *cli.CLI, opts releaseOptions) error {
	client, err := uncli.ConnectCluster(ctx, opts.cluster)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	domain, err := client.ReleaseDomain(ctx, &emptypb.Empty{})
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			return errors.New("no domain reserved")
		}
	}

	fmt.Printf("Released cluster domain: %s\n", domain.Name)
	return nil
}
