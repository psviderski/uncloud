package main

import (
	"context"
	"fmt"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/spf13/cobra"
)

// NewBuildCommand creates a new command to build services from a Compose file.
func NewBuildCommand() *cobra.Command {
	opts := cli.BuildOptions{}
	cmd := &cobra.Command{
		Use:   "build [FLAGS] [SERVICE...]",
		Short: "Build services from a Compose file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			if len(args) > 0 {
				opts.Services = args
			}

			return runBuild(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.Files, "file", "f", nil,
		"One or more Compose files to build (default compose.yaml)")
	cmd.Flags().StringSliceVarP(&opts.Profiles, "profile", "p", nil,
		"One or more Compose profiles to enable.")
	cmd.Flags().BoolVarP(&opts.Push, "push", "P", false,
		"Push built images to the registry after building. (default false)")
	cmd.Flags().BoolVarP(&opts.NoCache, "no-cache", "n", false,
		"Do not use cache when building images. (default false)")

	return cmd
}

// TODO: deduplicate with a similar functino for deploy options
// projectOpts returns the project options for the Compose file(s).
func projectOptsFromBuildOpts(opts cli.BuildOptions) []composecli.ProjectOptionsFn {
	projectOpts := []composecli.ProjectOptionsFn{}

	if len(opts.Profiles) > 0 {
		projectOpts = append(projectOpts, composecli.WithDefaultProfiles(opts.Profiles...))
	}

	return projectOpts
}

// runBuild parses the Compose file(s), builds the services, and pushes them if requested.
func runBuild(ctx context.Context, uncli *cli.CLI, opts cli.BuildOptions) error {
	projectOpts := projectOptsFromBuildOpts(opts)
	project, err := compose.LoadProject(ctx, opts.Files, projectOpts...)
	if err != nil {
		return fmt.Errorf("load compose file(s): %w", err)
	}

	if len(opts.Services) > 0 {
		project, err = project.WithSelectedServices(opts.Services)
		if err != nil {
			return fmt.Errorf("select services: %w", err)
		}
	}

	servicesToBuild := cli.GetServicesThatNeedBuild(project)

	if len(servicesToBuild) == 0 {
		fmt.Println("No services to build.")
		return nil
	}

	return cli.BuildServices(ctx, servicesToBuild, opts)
}
