// Package completion implements completion functions for the ucind cli.
package completion

import (
	"slices"
	"strings"

	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/spf13/cobra"
)

func Clusters(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	p := cmd.Context().Value("provisioner").(*ucind.Provisioner)
	clusters, err := p.ListClusters(cmd.Context())
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	names := []cobra.Completion{}
	for _, cluster := range clusters {
		if slices.Contains(args, cluster.Name) {
			continue
		}
		if strings.HasPrefix(cluster.Name, toComplete) {
			names = append(names, cluster.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
