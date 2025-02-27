package service

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"text/tabwriter"
	"uncloud/internal/cli"
)

func NewListCommand() *cobra.Command {
	var cluster string
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List services.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(cmd.Context(), uncli, cluster)
		},
	}
	cmd.Flags().StringVarP(
		&cluster, "cluster", "c", "",
		"Name of the cluster. (default is the current cluster)",
	)
	return cmd
}

func list(ctx context.Context, uncli *cli.CLI, clusterName string) error {
	client, err := uncli.ConnectCluster(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	services, err := client.ListServices(ctx)
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}

	// Print the list of services in a table format.
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	if _, err = fmt.Fprintln(tw, "NAME\tMODE\tREPLICAS\tENDPOINTS"); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, s := range services {
		endpointsSlice := s.Endpoints()
		endpoints := strings.Join(endpointsSlice, ", ")

		if _, err = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", s.Name, s.Mode, len(s.Containers), endpoints); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	return tw.Flush()
}
