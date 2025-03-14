package main

import (
	"context"
	"fmt"
	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/graph"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/spf13/cobra"
	"uncloud/internal/cli"
)

type deployOptions struct {
	files    []string
	services []string

	cluster string
}

// NewDeployCommand creates a new command to deploy services from a Compose file.
func NewDeployCommand() *cobra.Command {
	opts := deployOptions{}
	cmd := &cobra.Command{
		Use: "deploy [FLAGS] [SERVICE...]",
		// TODO: remove WIP when the command is fully implemented.
		Short: "WIP: Deploy services from a Compose file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			if len(args) > 0 {
				opts.services = args
			}

			return deploy(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.files, "file", "f", nil,
		"One or more Compose files to deploy services from. (default compose.yaml)")
	cmd.Flags().StringVarP(&opts.cluster, "cluster", "c", "",
		"Name of the cluster to deploy to (default is the current cluster)")

	return cmd
}

// deploy parses the Compose file(s) and deploys the services.
func deploy(ctx context.Context, uncli *cli.CLI, opts deployOptions) error {
	project, err := loadComposeProject(ctx, opts)
	if err != nil {
		return err
	}

	projectYAML, err := project.MarshalYAML()
	if err != nil {
		return err
	}

	fmt.Println(string(projectYAML))

	// TODO: move to ComposeDeployment.
	err = graph.InDependencyOrder(ctx, project,
		// TODO: properly handle dependency conditions.
		func(ctx context.Context, name string, service types.ServiceConfig) error {
			service, err := project.GetService(name)
			if err != nil {
				return err
			}

			fmt.Println(service.Name)

			return nil
		})

	return nil
}

func loadComposeProject(ctx context.Context, opts deployOptions) (*types.Project, error) {
	options, err := composecli.NewProjectOptions(
		opts.files,
		// First apply os.Environment, always wins.
		composecli.WithOsEnv,
		// Read dot env file to populate project environment.
		composecli.WithDotEnv,
		// Get compose file path set by COMPOSE_FILE.
		composecli.WithConfigFileEnv,
		// If none was selected, get default compose.yaml file from current dir or parent folders.
		composecli.WithDefaultConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf("create compose parser options: %w", err)
	}

	project, err := options.LoadProject(ctx)
	if err != nil {
		return nil, fmt.Errorf("load compose file(s): %w", err)
	}

	return project, nil
}
