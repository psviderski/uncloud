package completion

import (
	"context"
	"slices"
	"strings"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

func Volumes(ctx context.Context, uncli *cli.CLI, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	defer client.Close()

	volumes, err := client.ListVolumes(ctx, &api.VolumeFilter{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	names := []cobra.Completion{}
	for _, volume := range volumes {
		if slices.Contains(args, volume.Volume.Name) {
			continue
		}
		if strings.HasPrefix(volume.Volume.Name, toComplete) {
			names = append(names, volume.Volume.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
