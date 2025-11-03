package main

import (
	"context"
	"errors"
	"fmt"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	composeapi "github.com/docker/compose/v2/pkg/api"
	composev2 "github.com/docker/compose/v2/pkg/compose"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/spf13/cobra"
)

type buildOptions struct {
	buildArgs    []string
	check        bool
	deps         bool
	files        []string
	machines     []string
	noCache      bool
	profiles     []string
	pull         bool
	push         bool
	pushRegistry bool
	services     []string
	context      string
}

// NewCBuildCommand creates a new command to build images for services from a Compose file.
func NewCBuildCommand() *cobra.Command {
	opts := buildOptions{}
	cmd := &cobra.Command{
		Use:    "cbuild [FLAGS] [SERVICE...]",
		Short:  "Build services from a Compose file.",
		Long:   "Build images for services from a Compose file using Docker.",
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
		"Set a build-time variable for services. Used in Dockerfiles that declare the variable with ARG.\n"+
			"Can be specified multiple times. Format: --build-arg VAR=VALUE")
	cmd.Flags().BoolVar(&opts.check, "check", false,
		"Check the build configuration for services without building them.")
	cmd.Flags().BoolVar(&opts.deps, "deps", false,
		"Also build services declared as dependencies of the selected services.")
	cmd.Flags().StringSliceVarP(&opts.files, "file", "f", nil,
		"One or more Compose files to build. (default compose.yaml)")
	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Machine names or IDs to push the built images to (requires --push).\n"+
			"Can be specified multiple times or as a comma-separated list. (default is all machines)")
	cmd.Flags().BoolVar(&opts.noCache, "no-cache", false,
		"Do not use cache when building images.")
	cmd.Flags().StringSliceVarP(&opts.profiles, "profile", "p", nil,
		"One or more Compose profiles to enable.")
	cmd.Flags().BoolVar(&opts.pull, "pull", false,
		"Attempt to pull newer versions of the base images before building.")
	cmd.Flags().BoolVar(&opts.push, "push", false,
		"Upload the built images to cluster machines after building.\n"+
			"Use --machine to specify which machines. (default is all machines)")
	cmd.Flags().BoolVar(&opts.pushRegistry, "push-registry", false,
		"Upload the built images to registries after building.")
	cmd.Flags().StringVarP(
		&opts.context, "context", "c", "",
		"Name of the cluster context. (default is the current context)",
	)

	return cmd
}

// projectOptsFromCBuildOpts returns the project options for the Compose file(s).
func projectOptsFromCBuildOpts(opts buildOptions) []composecli.ProjectOptionsFn {
	var projOpts []composecli.ProjectOptionsFn

	if len(opts.profiles) > 0 {
		projOpts = append(projOpts, composecli.WithDefaultProfiles(opts.profiles...))
	}

	return projOpts
}

// runCBuild parses the Compose file(s) and builds the images for selected services.
func runCBuild(ctx context.Context, uncli *cli.CLI, opts buildOptions) error {
	// Validate push flags.
	if opts.push && opts.pushRegistry {
		return fmt.Errorf("cannot specify both --push and --push-registry: choose one push target")
	}

	projOpts := projectOptsFromCBuildOpts(opts)
	project, err := compose.LoadProject(ctx, opts.files, projOpts...)
	if err != nil {
		return fmt.Errorf("load compose file(s): %w", err)
	}

	servicesToBuild, err := cli.ServicesThatNeedBuild(project, opts.services, opts.deps)
	if err != nil {
		return fmt.Errorf("determine services to build: %w", err)
	}
	if len(servicesToBuild) == 0 {
		fmt.Println("No services to build.")
		return nil
	}

	// Build service images using Compose implementation.
	dockerCli, err := command.NewDockerCli()
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}

	// Initialise the Docker CLI with default options.
	if err = dockerCli.Initialize(flags.NewClientOptions()); err != nil {
		return fmt.Errorf("initialise docker client: %w", err)
	}

	composeService := composev2.NewComposeService(dockerCli)
	buildOpts := composeapi.BuildOptions{
		Args:     composetypes.NewMappingWithEquals(opts.buildArgs),
		Check:    opts.check,
		Deps:     opts.deps,
		NoCache:  opts.noCache,
		Pull:     opts.pull,
		Push:     opts.pushRegistry,
		Services: opts.services,
	}

	if err = composeService.Build(ctx, project, buildOpts); err != nil {
		return fmt.Errorf("build services: %w", err)
	}

	// Push images to cluster machines if --push is specified.
	if opts.push {
		if err = pushImagesToCluster(ctx, uncli, servicesToBuild, opts.machines); err != nil {
			return fmt.Errorf("push images to cluster: %w", err)
		}
	}

	return nil
}

// pushImagesToCluster pushes the locally built Docker images for specified services to cluster machines via unregistry.
func pushImagesToCluster(
	ctx context.Context,
	uncli *cli.CLI,
	services map[string]composetypes.ServiceConfig,
	machines []string,
) error {
	clusterClient, err := uncli.ConnectCluster(ctx, "")
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	machines = cli.ExpandCommaSeparatedValues(machines)
	pushOpts := client.PushImageOptions{}

	// Special handling for an explicit "all" keyword to push to all machines.
	if len(machines) == 1 && machines[0] == "all" {
		pushOpts.AllMachines = true
	} else if len(machines) > 0 {
		pushOpts.Machines = machines
	} else {
		// Default is to push to all machines in the cluster.
		pushOpts.AllMachines = true
	}

	// Push one service image at a time.
	var errs []error
	for _, s := range services {
		if s.Image == "" {
			// Skip services without an image name (shouldn't happen for services with build config).
			continue
		}

		err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
			if err = clusterClient.PushImage(ctx, s.Image, pushOpts); err != nil {
				return fmt.Errorf("push image for service '%s': %w", s.Name, err)
			}
			return nil
		}, uncli.ProgressOut(), fmt.Sprintf("Pushing image %s to cluster", s.Image))

		// Collect errors to try pushing all images.
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
