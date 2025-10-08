package compose

import (
	"context"
	"fmt"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
)

// FakeProjectName is a placeholder name for the project to be able to strip it from the resource names used as prefix.
const FakeProjectName = "f-a-k-e"

func LoadProject(ctx context.Context, paths []string, opts ...composecli.ProjectOptionsFn) (*types.Project, error) {
	defaultOpts := []composecli.ProjectOptionsFn{
		composecli.WithName(FakeProjectName),
		// First apply os.Environment, always wins.
		composecli.WithOsEnv,
		// Read dot env file to populate project environment.
		composecli.WithDotEnv,
		// Get compose file path set by COMPOSE_FILE.
		composecli.WithConfigFileEnv,
		// If none was selected, get default Compose file names from current or parent folders.
		composecli.WithDefaultConfigPath,
		composecli.WithExtension(CaddyExtensionKey, Caddy{}),
		composecli.WithExtension(MachinesExtensionKey, MachinesSource{}),
		composecli.WithExtension(PortsExtensionKey, PortsSource{}),
	}

	options, err := composecli.NewProjectOptions(
		paths,
		append(defaultOpts, opts...)...,
	)
	if err != nil {
		return nil, fmt.Errorf("create compose parser options: %w", err)
	}

	project, err := options.LoadProject(ctx)
	if err != nil {
		return nil, err
	}

	if project, err = transformServicesCaddyExtension(project); err != nil {
		return nil, err
	}
	if project, err = transformServicesPortsExtension(project); err != nil {
		return nil, err
	}

	// Validate extension combinations after all transformations.
	if err = validateServicesExtensions(project); err != nil {
		return nil, err
	}

	return project, nil
}
