package completion

import (
	"context"
	"slices"
	"strings"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func Machines(ctx context.Context, uncli *cli.CLI, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	defer client.Close()

	machines, err := client.ListMachines(ctx, nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	names := []cobra.Completion{}
	for _, machine := range machines {
		if slices.Contains(args, machine.Machine.Name) {
			continue
		}
		if strings.HasPrefix(machine.Machine.Name, toComplete) {
			names = append(names, machine.Machine.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func MachinesFlag(cmd *cobra.Command) {
	cmd.RegisterFlagCompletionFunc("machine",
		func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return Machines(cmd.Context(), uncli, args, toComplete)
		})
}
