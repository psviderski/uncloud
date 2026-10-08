package cert

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/caddyserver/certmagic"
	"github.com/docker/go-units"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
)

type listOptions struct {
	machine string
	output  string
}

func NewListCommand() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List certificates in cluster storage for Caddy.",
		Long: `List certificates stored in the Uncloud cluster storage for Caddy.

Caddy must use the Uncloud storage module (https://github.com/unlabs-dev/caddy-uncloud)
configured with 'storage uncloud' in the global options for its certificates to appear here.

This inventory does not check whether Caddy currently serves or trusts a certificate.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.machine, "machine", "m", "",
		"Name or ID of the machine to read certificate storage from. (default is connected machine)")
	cmd.Flags().StringVarP(&opts.output, "output", "o", "",
		"Output format: 'json' or empty for a human-readable table.")
	completion.MachinesFlag(cmd)

	return cmd
}

func list(ctx context.Context, uncli *cli.CLI, opts listOptions) error {
	if opts.output != "" && opts.output != "json" {
		return fmt.Errorf("unsupported output format '%s' (supported: json)", opts.output)
	}

	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	certs, listErr := clusterClient.Caddy.ListCertificates(ctx,
		client.CaddyListCertificatesOptions{Machine: opts.machine})
	if len(certs) > 0 || listErr == nil {
		if err = printCertificates(os.Stdout, certs, opts.output); err != nil {
			return fmt.Errorf("print Caddy certificates: %w", err)
		}
	}
	if listErr != nil {
		if len(certs) > 0 {
			return fmt.Errorf("certificate list is partial: %w", listErr)
		}
		return fmt.Errorf("list Caddy certificates: %w", listErr)
	}

	return nil
}

func printCertificates(out io.Writer, certs []api.IssuedCertificate, output string) error {
	type certificate struct {
		api.IssuedCertificate
		Fingerprint string
	}

	items := make([]certificate, 0, len(certs))
	for _, cert := range certs {
		fingerprint := cert.Fingerprint()
		items = append(items, certificate{
			IssuedCertificate: cert,
			Fingerprint:       hex.EncodeToString(fingerprint[:]),
		})
	}
	slices.SortFunc(items, func(a, b certificate) int {
		if cmp := strings.Compare(a.SAN, b.SAN); cmp != 0 {
			return cmp
		}
		if cmp := a.Chain[0].NotAfter.Compare(b.Chain[0].NotAfter); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Fingerprint, b.Fingerprint)
	})

	if output == "json" {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(items); err != nil {
			return fmt.Errorf("marshal certificates: %w", err)
		}
		return nil
	}

	if len(items) == 0 {
		_, err := fmt.Fprintln(out, "No Caddy certificates found in cluster storage.")
		return err
	}

	t := tui.NewTable()
	t.Headers("ID", "NAME", "ISSUER", "EXPIRES")
	for _, item := range items {
		t.Row(
			item.Fingerprint[:12],
			item.SAN,
			formatIssuer(item.IssuerData),
			formatExpiry(item.Chain[0].NotAfter),
		)
	}
	_, err := lipgloss.Fprintln(out, t)
	return err
}

func formatIssuer(data api.CertificateIssuerData) string {
	if data.ACME == nil {
		return "unknown"
	}

	switch data.ACME.CA {
	case certmagic.LetsEncryptProductionCA:
		return "Let's Encrypt"
	case certmagic.LetsEncryptStagingCA:
		return "Let's Encrypt (staging)"
	case certmagic.ZeroSSLProductionCA:
		return "ZeroSSL"
	case certmagic.GoogleTrustProductionCA:
		return "Google Trust Services"
	case certmagic.GoogleTrustStagingCA:
		return "Google Trust Services (staging)"
	default:
		return data.ACME.CA
	}
}

func formatExpiry(expires time.Time) string {
	delta := expires.Sub(time.Now())
	if delta <= 0 {
		return fmt.Sprintf("%s (expired %s ago)", expires.UTC().Format(time.DateOnly),
			strings.ToLower(units.HumanDuration(-delta)))
	}
	return fmt.Sprintf("%s (%s)", expires.UTC().Format(time.DateOnly),
		strings.ToLower(units.HumanDuration(delta)))
}
