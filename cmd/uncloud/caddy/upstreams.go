package caddy

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type upstreamsOptions struct {
	machine string
}

func NewUpstreamsCommand() *cobra.Command {
	opts := upstreamsOptions{}

	cmd := &cobra.Command{
		Use:   "upstreams",
		Short: "List Caddy upstreams and their health status.",
		Long:  "List Caddy upstreams and their health status from the connected machine or a specified one.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return runUpstreams(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.machine, "machine", "m", "",
		"Name or ID of the machine to get the upstreams from. (default is connected machine)")

	return cmd
}

func runUpstreams(ctx context.Context, uncli *cli.CLI, opts upstreamsOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	if opts.machine != "" {
		// If a specific machine is requested, use it to get the Caddy upstreams.
		ctx, _, err = api.ProxyMachinesContext(ctx, clusterClient, []string{opts.machine})
		if err != nil {
			return err
		}
	}

	resp, err := clusterClient.Caddy.GetUpstreams(ctx, nil)
	if err != nil {
		return fmt.Errorf("get Caddy upstreams: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "HOST\tUPSTREAMS")

	slices.SortFunc(resp.Hosts, func(a, b *pb.HostUpstreams) int {
		return strings.Compare(a.Host, b.Host)
	})

	for _, host := range resp.Hosts {
		var upstreamsStrs []string
		for _, u := range host.Upstreams {
			statusIcon := "✅"
			if u.Status != "healthy" {
				statusIcon = "❌"
			}

			// Format: 10.210.0.4:80 (✅ healthy, reqs: 0, fails: 0)
			s := fmt.Sprintf("%s (%s %s, reqs: %d, fails: %d)",
				u.Address, statusIcon, u.Status, u.NumRequests, u.Fails)
			upstreamsStrs = append(upstreamsStrs, s)
		}
		fmt.Fprintf(w, "%s\t%s\n", host.Host, strings.Join(upstreamsStrs, ", "))
	}
	return w.Flush()
}
