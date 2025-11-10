package dns

import (
	"context"
	"errors"
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func NewReleaseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Release the reserved cluster domain.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return release(cmd.Context(), uncli)
		},
	}

	return cmd
}

func release(ctx context.Context, uncli *cli.CLI) error {
	client, err := uncli.ConnectCluster(ctx)
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
