package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/spf13/cobra"
)

type deployOptions struct {
	files    []string
	profiles []string
	services []string
	noBuild  bool
	recreate bool

	context string
}

// NewDeployCommand creates a new command to deploy services from a Compose file.
func NewDeployCommand() *cobra.Command {
	opts := deployOptions{}
	cmd := &cobra.Command{
		Use:   "deploy [FLAGS] [SERVICE...]",
		Short: "Deploy services from a Compose file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			if len(args) > 0 {
				opts.services = args
			}

			return runDeploy(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.files, "file", "f", nil,
		"One or more Compose files to deploy services from. (default compose.yaml)")
	cmd.Flags().StringSliceVarP(&opts.profiles, "profile", "p", nil,
		"One or more Compose profiles to enable.")
	cmd.Flags().StringVarP(&opts.context, "context", "c", "",
		"Name of the cluster context to deploy to (default is the current context)")
	cmd.Flags().BoolVarP(&opts.noBuild, "no-build", "n", false,
		"Do not build images before deploying services. (default false)")
	cmd.Flags().BoolVar(&opts.recreate, "recreate", false,
		"Recreate containers even if their configuration and image haven't changed.")

	// TODO: Consider adding a filter flag to specify which machines to deploy to but keep the rest running.
	//  Could be useful to test a new version on a subset of machines before rolling out to all.

	return cmd
}

// projectOpts returns the project options for the Compose file(s).
func projectOpts(opts deployOptions) []composecli.ProjectOptionsFn {
	projectOpts := []composecli.ProjectOptionsFn{}

	if len(opts.profiles) > 0 {
		projectOpts = append(projectOpts, composecli.WithDefaultProfiles(opts.profiles...))
	}

	return projectOpts
}

// runDeploy parses the Compose file(s) and deploys the services.
func runDeploy(ctx context.Context, uncli *cli.CLI, opts deployOptions) error {
	projectOpts := projectOpts(opts)

	project, err := compose.LoadProject(ctx, opts.files, projectOpts...)
	if err != nil {
		return fmt.Errorf("load compose file(s): %w", err)
	}

	if len(opts.services) > 0 {
		// Includes service dependencies by default. This is the default docker compose behavior.
		project, err = project.WithSelectedServices(opts.services)
		if err != nil {
			return fmt.Errorf("select services: %w", err)
		}
	}

	servicesToBuild := cli.GetServicesThatNeedBuild(project)

	if len(servicesToBuild) > 0 {
		if opts.noBuild {
			fmt.Println("Not building services as requested.")
		} else {
			buildOpts := cli.BuildOptions{
				Push:    true,
				NoCache: false,
			}

			if err := cli.BuildServices(ctx, servicesToBuild, buildOpts); err != nil {
				return fmt.Errorf("build services: %w", err)
			}
		}
	}

	clusterClient, err := uncli.ConnectCluster(ctx, opts.context)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	var strategy deploy.Strategy
	if opts.recreate {
		strategy = &deploy.RollingStrategy{ForceRecreate: true}
	}
	composeDeploy, err := compose.NewDeploymentWithStrategy(ctx, clusterClient, project, strategy)
	if err != nil {
		return fmt.Errorf("create compose deployment: %w", err)
	}

	plan, err := composeDeploy.Plan(ctx)
	if err != nil {
		return fmt.Errorf("plan deployment: %w", err)
	}

	if len(plan.Operations) == 0 {
		fmt.Println("Services are up to date.")
		return nil
	}

	fmt.Println("Deployment plan:")
	if err = printPlan(ctx, clusterClient, plan); err != nil {
		return fmt.Errorf("print deployment plan: %w", err)
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

func printPlan(ctx context.Context, cli *client.Client, plan deploy.SequenceOperation) error {
	for _, op := range plan.Operations {
		svcPlan, ok := op.(*deploy.Plan)
		if !ok {
			fmt.Println("- " + op.Format(nil))
			continue
		}

		svc, err := cli.InspectService(ctx, svcPlan.ServiceID)
		if err != nil && !errors.Is(err, api.ErrNotFound) {
			return fmt.Errorf("inspect service: %w", err)
		}
		// Initialise a machine and container name resolver to properly format the service plan output.
		resolver, err := cli.ServiceOperationNameResolver(ctx, svc)
		if err != nil {
			return fmt.Errorf("create machine and container name resolver for service operations: %w", err)
		}

		fmt.Printf("- Deploy service [name=%s]\n", svcPlan.ServiceName)
		fmt.Println(indent(svcPlan.Format(resolver), "  "))
	}

	return nil
}

func indent(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
