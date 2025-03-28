package compose

// TODO: make compose, cli, and api packages public.

import (
	"context"
	"fmt"
	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
)

func LoadProject(ctx context.Context, paths []string) (*types.Project, error) {
	options, err := composecli.NewProjectOptions(
		paths,
		// First apply os.Environment, always wins.
		composecli.WithOsEnv,
		// Read dot env file to populate project environment.
		composecli.WithDotEnv,
		// Get compose file path set by COMPOSE_FILE.
		composecli.WithConfigFileEnv,
		// If none was selected, get default compose-ports-long.yaml file from current dir or parent folders.
		composecli.WithDefaultConfigPath,
		composecli.WithExtension(PortsExtensionKey, PortsSource{}),
	)
	if err != nil {
		return nil, fmt.Errorf("create compose parser options: %w", err)
	}

	project, err := options.LoadProject(ctx)
	if err != nil {
		return nil, err
	}

	if project, err = transformServicesPortsExtension(project); err != nil {
		return nil, err
	}

	return project, nil
}
