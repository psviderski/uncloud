package dns

import (
	"context"
	"errors"
	"fmt"

	"github.com/miekg/dns"
	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func NewSetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set DOMAIN_NAME",
		Args:  cobra.ExactArgs(1),
		Short: "Set or unset an externally managed cluster domain (EXPERIMENTAL).",
		Long: "EXPERIMENTAL: Set the cluster domain used to generate ingress hostnames for services.\n" +
			"Configure wildcard DNS records for this domain with your DNS provider. " +
			"Uncloud will not create, verify, update, or delete external DNS records.\n" +
			"Pass an empty string to unset a manually set domain. " +
			"Use 'uc dns release' to release a domain reserved in Uncloud DNS. " +
			"Setting or unsetting the domain does not change existing service hostnames.",
		Example: "  uc dns set apps.example.com\n  uc dns set \"\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return set(cmd.Context(), uncli, args[0])
		},
	}

	return cmd
}

func set(ctx context.Context, uncli *cli.CLI, name string) error {
	if name != "" {
		if labels, ok := dns.IsDomainName(name); !ok || labels < 2 {
			return fmt.Errorf("invalid cluster domain %q: must be a valid domain name with at least two labels", name)
		}
	}
	tui.PrintWarning("Setting an externally managed cluster domain is experimental.")

	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	_, err = clusterClient.SetDomain(ctx, &pb.SetDomainRequest{Name: name})
	if err != nil {
		if status.Convert(err).Code() == codes.AlreadyExists {
			return errors.New("cluster domain already configured, unset it with 'uc dns set \"\"' or " +
				"release a reservation with 'uc dns release' first")
		}
		return fmt.Errorf("set cluster domain: %w", err)
	}

	if name == "" {
		fmt.Println("Unset cluster domain. External DNS records have not been changed.")
		return nil
	}
	fmt.Printf("Set cluster domain: %s\n", name)
	fmt.Println("DNS is managed externally. " +
		"Configure wildcard DNS records pointing to machines running the Caddy service.")
	return nil
}
