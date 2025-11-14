package dns

import (
	"context"
	"errors"
	"fmt"

	"github.com/psviderski/uncloud/cmd/uncloud/caddy"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const DefaultUncloudDNSAPIEndpoint = "https://dns.uncloud.run/v1"

type reserveOptions struct {
	endpoint string
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

	return cmd
}

func reserve(ctx context.Context, uncli *cli.CLI, opts reserveOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	domain, err := clusterClient.ReserveDomain(ctx, &pb.ReserveDomainRequest{Endpoint: opts.endpoint})
	if err != nil {
		if status.Convert(err).Code() == codes.AlreadyExists {
			return errors.New("domain already reserved")
		}
		return err
	}

	fmt.Printf("Reserved cluster domain: %s\n", domain.Name)

	// Update cluster domain records in Uncloud DNS to point to machines running caddy service if it has been deployed.
	if _, err = clusterClient.InspectService(ctx, client.CaddyServiceName); err != nil {
		if errors.Is(err, api.ErrNotFound) {
			fmt.Println("Deploy the Caddy reverse proxy service ('uc caddy deploy') to enable internet access " +
				"to your services via the reserved or your custom domain.")
			return nil
		}
		return fmt.Errorf("inspect caddy service: %w", err)
	}

	return caddy.UpdateDomainRecords(ctx, clusterClient, uncli.ProgressOut())
}
