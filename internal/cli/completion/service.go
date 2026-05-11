// Package completion implements completion functions for the uc cli.
package completion

import (
	"context"
	"slices"
	"strings"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func Services(ctx context.Context, uncli *cli.CLI, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	defer client.Close()

	services, err := client.ListServices(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	names := []cobra.Completion{}
	for _, service := range services {
		if slices.Contains(args, service.Name) {
			continue
		}
		if strings.HasPrefix(service.Name, toComplete) {
			names = append(names, service.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
