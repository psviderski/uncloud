package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	composeapi "github.com/docker/compose/v2/pkg/api"
	composev2 "github.com/docker/compose/v2/pkg/compose"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/compose"
)

// BuildServicesOptions contains options for building services in a Compose project.
type BuildServicesOptions struct {
	// BuildArgs sets build-time variables for services. Used in Dockerfiles that declare variables with ARG.
	BuildArgs []string
	// Check the build configuration for services without building them.
	Check bool
	// Deps enables to also build services declared as dependencies of the selected Services.
	Deps bool
	// NoCache disables the use of cache when building images.
	NoCache bool
	// Pull attempts to pull newer versions of the base images before building.
	Pull bool
	// Services specifies which services to build. If empty, all services with a build config are built.
	Services []string

	// Push targets are mutually exclusive.
	// PushCluster uploads the built images to cluster machines after building.
	PushCluster bool
	// PushRegistry uploads the built images to external registries after building.
	PushRegistry bool

	// Cluster-specific options (only used if PushCluster is true).
	// Machines is a list of machine names or IDs to push the image to. If empty, images are pushed to all machines.
	Machines []string
}

// BuildServices builds images for services in the Compose project.
func (cli *CLI) BuildServices(ctx context.Context, project *composetypes.Project, opts BuildServicesOptions) error {
	// Validate push targets.
	if opts.PushCluster && opts.PushRegistry {
		return fmt.Errorf("cannot specify both PushCluster and PushRegistry: choose one push target")
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
		Args:     composetypes.NewMappingWithEquals(opts.BuildArgs),
		Check:    opts.Check,
		Deps:     opts.Deps,
		NoCache:  opts.NoCache,
		Pull:     opts.Pull,
		Push:     opts.PushRegistry,
		Services: opts.Services,
	}

	if err = composeService.Build(ctx, project, buildOpts); err != nil {
		return err
	}

	if !opts.PushCluster {
		return nil
	}

	// Push built service images to cluster machines.
	builtServices, err := ServicesThatNeedBuild(project, opts.Services, opts.Deps)
	if err != nil {
		return fmt.Errorf("determine built services: %w", err)
	}
	if len(builtServices) == 0 {
		// No services were built, nothing to push.
		return nil
	}

	// Add a line break after the build output.
	fmt.Fprintln(cli.ProgressOut())

	clusterClient, err := cli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	// Push one service image at a time.
	var errs []error
	for _, s := range builtServices {
		if s.Image == "" {
			// Skip services without an image name (shouldn't happen for services with build config).
			continue
		}

		// Push to the specified machines falling back to service x-machines.
		// If none specified, push to *all* cluster machines.
		var pushOpts client.PushImageOptions

		if len(opts.Machines) > 0 {
			pushOpts.Machines = opts.Machines
		} else if machines, ok := s.Extensions[compose.MachinesExtensionKey].(compose.MachinesSource); ok {
			pushOpts.Machines = machines
		}

		if len(pushOpts.Machines) == 0 {
			pushOpts.AllMachines = true
		}

		boldStyle := lipgloss.NewStyle().Bold(true)
		err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
			if err = clusterClient.PushImage(ctx, s.Image, pushOpts); err != nil {
				return fmt.Errorf("push image '%s' for service '%s': %w", s.Image, s.Name, err)
			}
			return nil
		}, cli.ProgressOut(), fmt.Sprintf("Pushing image %s to cluster", boldStyle.Render(s.Image)))
		// Collect errors to try pushing all images.
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// ServicesThatNeedBuild returns a list of services that require building.
// deps indicates whether to include services that are dependencies of the selected services.
// Implementation is based on the logic from docker/compose/v2/pkg/compose/build.go.
func ServicesThatNeedBuild(
	project *composetypes.Project, selectedServices []string, deps bool,
) ([]composetypes.ServiceConfig, error) {
	servicesToBuild := make([]composetypes.ServiceConfig, 0, len(project.Services))

	var policy composetypes.DependencyOption = composetypes.IgnoreDependencies
	if deps {
		policy = composetypes.IncludeDependencies
	}

	// Also include services used as build.additional_contexts with service: prefix.
	selectedServices = includeAdditionalContextsServices(project, selectedServices)
	// Some build dependencies we just introduced may not be enabled, enable them.
	var err error
	if project, err = project.WithServicesEnabled(selectedServices...); err != nil {
		return nil, err
	}
	if project, err = project.WithSelectedServices(selectedServices); err != nil {
		return nil, err
	}

	err = project.ForEachService(selectedServices, func(serviceName string, service *composetypes.ServiceConfig) error {
		if service.Build != nil {
			servicesToBuild = append(servicesToBuild, *service)
		}
		return nil
	}, policy)
	if err != nil {
		return nil, err
	}

	return servicesToBuild, nil
}

// includeAdditionalContextsServices adds services referenced in build.additional_contexts to the list
// of selected services.
func includeAdditionalContextsServices(project *composetypes.Project, selectedServices []string) []string {
	servicesWithDependencies := mapset.NewSet(selectedServices...)
	for _, service := range selectedServices {
		s, ok := project.Services[service]
		if !ok {
			s = project.DisabledServices[service]
		}
		if s.Build != nil {
			for _, target := range s.Build.AdditionalContexts {
				if name, found := strings.CutPrefix(target, composetypes.ServicePrefix); found {
					servicesWithDependencies.Add(name)
				}
			}
		}
	}
	if servicesWithDependencies.Cardinality() > len(selectedServices) {
		return includeAdditionalContextsServices(project, servicesWithDependencies.ToSlice())
	}

	return servicesWithDependencies.ToSlice()
}
