package dns

import (
	"context"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"uncloud/internal/cli"
	"uncloud/internal/machine/api/pb"
)

const DefaultUncloudDNSAPIEndpoint = "https://dns.uncloud.run/v1"

type reserveOptions struct {
	endpoint string
	cluster  string
}

func NewReserveCommand() *cobra.Command {
	opts := reserveOptions{}

	cmd := &cobra.Command{
		Use:   "reserve",
		Short: "Reserve a cluster domain in Uncloud DNS.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return reserve(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringVar(&opts.endpoint, "endpoint", DefaultUncloudDNSAPIEndpoint,
		"API endpoint for the Uncloud DNS service.")
	cmd.Flags().StringVarP(
		&opts.cluster, "cluster", "c", "",
		"Name of the cluster. (default is the current cluster)",
	)

	return cmd
}

func reserve(ctx context.Context, uncli *cli.CLI, opts reserveOptions) error {
	client, err := uncli.ConnectCluster(ctx, opts.cluster)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	domain, err := client.ReserveDomain(ctx, &pb.ReserveDomainRequest{Endpoint: opts.endpoint})
	if err != nil {
		if status.Convert(err).Code() == codes.AlreadyExists {
			return errors.New("domain already reserved")
		}
		return err
	}

	fmt.Printf("Reserved cluster domain: %s\n", domain.Name)
	fmt.Println("Redeploy the Caddy service ('uc caddy deploy') to configure DNS records for the domain " +
		"to route traffic to the services in the cluster.")
	return nil
}
