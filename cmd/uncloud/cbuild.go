package main

import (
	"context"
	"fmt"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	composeapi "github.com/docker/compose/v2/pkg/api"
	composev2 "github.com/docker/compose/v2/pkg/compose"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/spf13/cobra"
)

type buildOptions struct {
	buildArgs []string
	files     []string
	noCache   bool
	profiles  []string
	pull      bool
	services  []string
}

// NewCBuildCommand creates a new command to build images for services from a Compose file.
func NewCBuildCommand() *cobra.Command {
	opts := buildOptions{}
	cmd := &cobra.Command{
		Use:    "cbuild [FLAGS] [SERVICE...]",
		Short:  "Build services from a Compose file.",
		Long:   "Build images for services from a Compose file using the Docker Compose library.",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			if len(args) > 0 {
				opts.services = args
			}

			return runCBuild(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringArrayVar(&opts.buildArgs, "build-arg", nil,
		"Set a build-time variable to pass to service Dockerfiles. Only used if declared with ARG.\n"+
			"Can be specified multiple times. Format: --build-arg VAR=VALUE")
	cmd.Flags().StringSliceVarP(&opts.files, "file", "f", nil,
		"One or more Compose files to build (default compose.yaml)")
	cmd.Flags().BoolVar(&opts.noCache, "no-cache", false,
		"Do not use cache when building images.")
	cmd.Flags().StringSliceVarP(&opts.profiles, "profile", "p", nil,
		"One or more Compose profiles to enable.")
	cmd.Flags().BoolVar(&opts.pull, "pull", false,
		"Attempt to pull newer versions of the base images before building.")

	return cmd
}

// projectOptsFromCBuildOpts returns the project options for the Compose file(s).
func projectOptsFromCBuildOpts(opts buildOptions) []composecli.ProjectOptionsFn {
	var projectOpts []composecli.ProjectOptionsFn

	if len(opts.profiles) > 0 {
		projectOpts = append(projectOpts, composecli.WithDefaultProfiles(opts.profiles...))
	}

	return projectOpts
}

// runCBuild parses the Compose file(s) and builds the images for selected services.
func runCBuild(ctx context.Context, _ *cli.CLI, opts buildOptions) error {
	projectOpts := projectOptsFromCBuildOpts(opts)
	project, err := compose.LoadProject(ctx, opts.files, projectOpts...)
	if err != nil {
		return fmt.Errorf("load compose file(s): %w", err)
	}

	if len(opts.services) > 0 {
		project, err = project.WithSelectedServices(opts.services)
		if err != nil {
			return fmt.Errorf("select services: %w", err)
		}
	}

	servicesToBuild := cli.GetServicesThatNeedBuild(project)

	if len(servicesToBuild) == 0 {
		fmt.Println("No services to build.")
		return nil
	}

	// Build service images using Compose implementation.
	dockerCli, err := command.NewDockerCli()
	if err != nil {
		return fmt.Errorf("create docker CLI: %w", err)
	}

	// Initialise the Docker CLI with default options.
	if err = dockerCli.Initialize(flags.NewClientOptions()); err != nil {
		return fmt.Errorf("initialize docker CLI: %w", err)
	}

	composeService := composev2.NewComposeService(dockerCli)
	buildOpts := composeapi.BuildOptions{
		Args:     composetypes.NewMappingWithEquals(opts.buildArgs),
		NoCache:  opts.noCache,
		Pull:     opts.pull,
		Services: opts.services,
	}

	if err = composeService.Build(ctx, project, buildOpts); err != nil {
		return fmt.Errorf("build services: %w", err)
	}

	fmt.Println("Build completed successfully.")

	return nil
}
