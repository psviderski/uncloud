package dns

import (
	"context"
	"errors"
	"fmt"

	"github.com/miekg/dns"
	"github.com/psviderski/uncloud/cmd/uc/caddy"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func NewSetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set DOMAIN_NAME",
		Args:  cobra.ExactArgs(1),
		Short: "Set a cluster domain directly in the cluster.",
		Long: "Set a cluster domain directly in the cluster, bypassing Uncloud DNS. " +
			"This assumes the DNS is externally set up.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return set(cmd.Context(), uncli, args[0])
		},
	}

	return cmd
}

func set(ctx context.Context, uncli *cli.CLI, name string) error {
	labels, ok := dns.IsDomainName(name)
	if !ok || labels < 3 {
		return fmt.Errorf("domain '%s' is not a valid name", name)
	}

	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	domain, err := clusterClient.SetDomain(ctx, &pb.SetDomainRequest{Name: name})
	if err != nil {
		if status.Convert(err).Code() == codes.AlreadyExists {
			return errors.New("domain already reserved")
		}
		return err
	}

	fmt.Printf("Set cluster domain: %s\n", domain.Name)

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
