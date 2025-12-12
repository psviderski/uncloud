package main

import (
	"context"
	"fmt"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/spf13/cobra"
)

type buildOptions struct {
	cli.BuildServicesOptions
	files    []string
	profiles []string
}

// NewBuildCommand creates a new command to build images for services from a Compose file.
func NewBuildCommand() *cobra.Command {
	opts := buildOptions{}
	cmd := &cobra.Command{
		Use:   "build [FLAGS] [SERVICE...]",
		Short: "Build services from a Compose file.",
		Long: `Build images for services from a Compose file using local Docker.

By default, built images remain on the local Docker host. Use --push to upload them
to cluster machines or --push-registry to upload them to external registries.`,
		Example: `  # Build all services that have a build section in compose.yaml.
  uc build

  # Build specific services that have a build section.
  uc build web api

  # Build services and push images to all cluster machines or service x-machines if specified.
  uc build --push

  # Build services and push images to specific machines.
  uc build --push -m machine1,machine2

  # Build services and push images to external registries (e.g., Docker Hub).
  uc build --push-registry

  # Build services with build arguments, pull newer base images before building, and don't use cache.
  uc build --build-arg NODE_VERSION=24 --build-arg ENV=production --no-cache --pull`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			opts.Services = args

			return runBuild(cmd.Context(), uncli, opts)
		},
		GroupID: "service",
	}

	cmd.Flags().StringArrayVar(&opts.BuildArgs, "build-arg", nil,
		"Set a build-time variable for services. Used in Dockerfiles that declare the variable with ARG.\n"+
			"Can be specified multiple times. Format: --build-arg VAR=VALUE")
	cmd.Flags().BoolVar(&opts.Check, "check", false,
		"Check the build configuration for services without building them.")
	cmd.Flags().BoolVar(&opts.Deps, "deps", false,
		"Also build services declared as dependencies of the selected services.")
	cmd.Flags().StringSliceVarP(&opts.files, "file", "f", nil,
		"One or more Compose files to build. (default compose.yaml)")
	cmd.Flags().StringSliceVarP(&opts.Machines, "machine", "m", nil,
		"Machine names or IDs to push the built images to (requires --push).\n"+
			"Can be specified multiple times or as a comma-separated list. (default is all machines or x-machines)")
	cmd.Flags().BoolVar(&opts.NoCache, "no-cache", false,
		"Do not use cache when building images.")
	cmd.Flags().StringSliceVarP(&opts.profiles, "profile", "p", nil,
		"One or more Compose profiles to enable.")
	cmd.Flags().BoolVar(&opts.Pull, "pull", false,
		"Always attempt to pull newer versions of base images before building.")
	cmd.Flags().BoolVar(&opts.PushCluster, "push", false,
		"Upload the built images to cluster machines after building.\n"+
			"Use --machine to specify which machines. (default is all machines)")
	cmd.Flags().BoolVar(&opts.PushRegistry, "push-registry", false,
		"Upload the built images to external registries (e.g., Docker Hub) after building.")

	return cmd
}

// runBuild parses the Compose file(s) and builds the images for selected services.
func runBuild(ctx context.Context, uncli *cli.CLI, opts buildOptions) error {
	// Validate push flags.
	if opts.PushCluster && opts.PushRegistry {
		return fmt.Errorf("cannot specify both --push and --push-registry: choose one push target")
	}

	machines := cli.ExpandCommaSeparatedValues(opts.Machines)
	// Special handling for an explicit "all" keyword to push to all machines.
	if len(machines) == 1 && machines[0] == "all" {
		machines = nil
	}
	opts.Machines = machines

	project, err := compose.LoadProject(ctx, opts.files, composecli.WithDefaultProfiles(opts.profiles...))
	if err != nil {
		return fmt.Errorf("load compose file(s): %w", err)
	}

	servicesToBuild, err := cli.ServicesThatNeedBuild(project, opts.Services, opts.Deps)
	if err != nil {
		return fmt.Errorf("determine services to build: %w", err)
	}
	if len(servicesToBuild) == 0 {
		fmt.Println("No services to build.")
		return nil
	}

	return uncli.BuildServices(ctx, project, opts.BuildServicesOptions)
}
