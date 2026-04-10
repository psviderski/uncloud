// Package completion implements completion functions for the uc cli.
package completion

import (
	"context"
	"strings"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func Services(ctx context.Context, uncli *cli.CLI, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	services, err := client.ListServices(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	names := []cobra.Completion{}
	for _, service := range services {
		if strings.HasPrefix(service.Name, toComplete) {
			names = append(names, service.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
