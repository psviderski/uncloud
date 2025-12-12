package service

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

func NewListCommand(groupID string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List services.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(cmd.Context(), uncli)
		},
		GroupID: groupID,
	}
	return cmd
}

func list(ctx context.Context, uncli *cli.CLI) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	services, err := client.ListServices(ctx)
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}

	// Sort services by name.
	haveDuplicateNames := false
	slices.SortFunc(services, func(a, b api.Service) int {
		if a.Name == b.Name {
			haveDuplicateNames = true
			return strings.Compare(a.ID, b.ID)
		}
		return strings.Compare(a.Name, b.Name)
	})

	// Print the list of services in a table format.
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

	// Include the ID column if there are duplicate service names to differentiate them.
	if haveDuplicateNames {
		if _, err = fmt.Fprintf(tw, "ID\t"); err != nil {
			return fmt.Errorf("write header: %w", err)
		}
	}
	if _, err = fmt.Fprintln(tw, "NAME\tMODE\tREPLICAS\tIMAGE\tENDPOINTS"); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, s := range services {
		images := strings.Join(s.Images(), ", ")
		endpoints := strings.Join(s.Endpoints(), ", ")

		// If no endpoints from ports, check if the service uses custom Caddy config.
		if endpoints == "" {
			for _, ctr := range s.Containers {
				if ctr.Container.ServiceSpec.CaddyConfig() != "" {
					endpoints = "(custom Caddy config)"
				}
			}
		}

		if haveDuplicateNames {
			if _, err = fmt.Fprintf(tw, "%s\t", s.ID); err != nil {
				return fmt.Errorf("write row: %w", err)
			}
		}
		if _, err = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n",
			s.Name, s.Mode, len(s.Containers), images, endpoints); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	return tw.Flush()
}
