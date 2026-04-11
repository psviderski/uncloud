package completion

import (
	"context"
	"slices"
	"strings"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/spf13/cobra"
)

func ComposeServices(ctx context.Context, args []string, toComplete string, files, profiles []string) ([]cobra.Completion, cobra.ShellCompDirective) {
	project, err := compose.LoadProject(ctx, files, composecli.WithDefaultProfiles(profiles...))
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	services := project.ServiceNames()
	names := []cobra.Completion{}
	for _, service := range services {
		if slices.Contains(args, service) {
			continue
		}
		if strings.HasPrefix(service, toComplete) {
			names = append(names, service)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
