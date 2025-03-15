package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/spf13/cobra"
	"strings"
	"uncloud/internal/cli"
	"uncloud/internal/cli/client"
	"uncloud/internal/compose"
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
	project, err := compose.LoadProject(ctx, opts.files)
	if err != nil {
		return fmt.Errorf("load compose file(s): %w", err)
	}

	if len(opts.services) > 0 {
		// TODO: handle dependencies properly.
		project, err = project.WithSelectedServices(opts.services, types.IgnoreDependencies)
		if err != nil {
			return fmt.Errorf("select services: %w", err)
		}
	}

	clusterClient, err := uncli.ConnectCluster(ctx, opts.cluster)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	plan, err := client.PlanComposeDeployment(ctx, project, clusterClient)
	if err != nil {
		return fmt.Errorf("plan deployment: %w", err)
	}

	if len(plan.Operations) == 0 {
		fmt.Println("Services are up to date.")
		return nil
	}

	fmt.Println("Deployment plan:")

	for _, op := range plan.Operations {
		svcPlan, ok := op.(*client.Plan)
		if !ok {
			return fmt.Errorf("expected service Plan, got: %T", op)
		}

		svc, err := clusterClient.InspectService(ctx, svcPlan.ServiceID)
		if err != nil {
			if !errors.Is(err, client.ErrNotFound) {
				return fmt.Errorf("inspect service: %w", err)
			}
			fmt.Printf("- Run service [name=%s]\n", svcPlan.ServiceName)
		} else {
			fmt.Printf("- Update service [name=%s]\n", svc.Name)
		}

		// Initialise a machine and container name resolver to properly format the service plan output.
		resolver, err := clusterClient.ServiceOperationNameResolver(ctx, svc)
		if err != nil {
			return fmt.Errorf("create machine and container name resolver for service operations: %w", err)
		}

		fmt.Println(indent(svcPlan.Format(resolver), "  "))
	}
	fmt.Println()

	confirmed, err := cli.Confirm()
	if err != nil {
		return fmt.Errorf("confirm deployment: %w", err)
	}
	if !confirmed {
		fmt.Println("Cancelled. No changes were made.")
		return nil
	}

	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		if err := plan.Execute(ctx, clusterClient); err != nil {
			return fmt.Errorf("deploy services: %w", err)
		}
		return nil
	}, uncli.ProgressOut(), "Deploying services")
}

func indent(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
