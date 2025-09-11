package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func NewListCommand() *cobra.Command {
	var contextName string
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List services.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(cmd.Context(), uncli, contextName)
		},
	}
	cmd.Flags().StringVarP(
		&contextName, "context", "c", "",
		"Name of the cluster context. (default is the current context)",
	)
	return cmd
}

func list(ctx context.Context, uncli *cli.CLI, contextName string) error {
	client, err := uncli.ConnectCluster(ctx, contextName)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	services, err := client.ListServices(ctx)
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}

	serviceNames := make(map[string]struct{}, len(services))
	haveDuplicateNames := false
	for _, svc := range services {
		if _, exists := serviceNames[svc.Name]; exists {
			haveDuplicateNames = true
			break
		}
		serviceNames[svc.Name] = struct{}{}
	}

	// Print the list of services in a table format.
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

	// Include the ID column if there are duplicate service names to differentiate them.
	if haveDuplicateNames {
		if _, err = fmt.Fprintf(tw, "ID\t"); err != nil {
			return fmt.Errorf("write header: %w", err)
		}
	}
	if _, err = fmt.Fprintln(tw, "NAME\tMODE\tREPLICAS\tENDPOINTS"); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, s := range services {
		endpointsSlice := s.Endpoints()
		endpoints := strings.Join(endpointsSlice, ", ")

		if haveDuplicateNames {
			if _, err = fmt.Fprintf(tw, "%s\t", s.ID); err != nil {
				return fmt.Errorf("write row: %w", err)
			}
		}
		if _, err = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", s.Name, s.Mode, len(s.Containers), endpoints); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	return tw.Flush()
}
