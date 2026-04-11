package completion

import (
	"context"
	"maps"
	"slices"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func Contexts(ctx context.Context, uncli *cli.CLI, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	contexts := slices.Sorted(maps.Keys(uncli.Config.Contexts))

	names := []cobra.Completion{}
	for _, context := range contexts {
		if slices.Contains(args, context) {
			continue
		}
		names = append(names, context)
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}
