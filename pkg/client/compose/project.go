package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
)

// LoadProject loads a Compose project from the default locations or the given paths.
func LoadProject(ctx context.Context, paths []string, opts ...composecli.ProjectOptionsFn) (*types.Project, error) {
	defaultOpts := []composecli.ProjectOptionsFn{
		// First apply os.Environment, always wins.
		composecli.WithOsEnv,
		// Set the local .env file to be loaded by WithDotEnv. COMPOSE_DISABLE_ENV_FILE can disable it.
		composecli.WithEnvFiles(),
		// Read environment variables from .env files set by WithEnvFiles (.env by default) to make available
		// for interpolation.
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

	removeProjectPrefixFromNames(project)
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

	// Process image templates in services to expand Go template expressions using git repo state.
	if project, err = ProcessImageTemplates(project); err != nil {
		return nil, err
	}

	return project, nil
}

// LoadProjectFromContent loads a Compose project from the given YAML content.
func LoadProjectFromContent(
	ctx context.Context, content string, opts ...composecli.ProjectOptionsFn,
) (*types.Project, error) {
	// Create a temporary directory for the compose file.
	tmpDir, err := os.MkdirTemp("", "uncloud-compose-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write the YAML content to compose.yaml in the temporary directory.
	composePath := filepath.Join(tmpDir, "compose.yaml")
	if err := os.WriteFile(composePath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("write compose file: %w", err)
	}

	return LoadProject(ctx, []string{composePath}, opts...)
}

// removeProjectPrefixFromNames removes the project name prefix from volume names.
func removeProjectPrefixFromNames(project *types.Project) {
	prefix := project.Name + "_"
	for name, vol := range project.Volumes {
		vol.Name = strings.TrimPrefix(vol.Name, prefix)
		project.Volumes[name] = vol
	}
}
