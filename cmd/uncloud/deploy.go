package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/psviderski/uncloud/pkg/client/deploy/operation"
	"github.com/spf13/cobra"
)

type deployOptions struct {
	cli.BuildServicesOptions

	files    []string
	profiles []string
	services []string
	noBuild  bool
	recreate bool
	yes      bool
}

// NewDeployCommand creates a new command to deploy services from a Compose file.
func NewDeployCommand() *cobra.Command {
	opts := deployOptions{}
	cmd := &cobra.Command{
		Use:   "deploy [FLAGS] [SERVICE...]",
		Short: "Deploy services from a Compose file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.BindEnvToFlag(cmd, "yes", "UNCLOUD_AUTO_CONFIRM")

			uncli := cmd.Context().Value("cli").(*cli.CLI)
			opts.services = args

			return runDeploy(cmd.Context(), uncli, opts)
		},
		GroupID: "service",
	}

	cmd.Flags().StringArrayVar(&opts.BuildServicesOptions.BuildArgs, "build-arg", nil,
		"Set a build-time variable for services. Used in Dockerfiles that declare the variable with ARG.\n"+
			"Can be specified multiple times. Format: --build-arg VAR=VALUE")
	cmd.Flags().BoolVar(&opts.BuildServicesOptions.Pull, "build-pull", false,
		"Always attempt to pull newer versions of base images before building service images.")
	cmd.Flags().StringSliceVarP(&opts.files, "file", "f", nil,
		"One or more Compose files to deploy services from. (default compose.yaml)")
	cmd.Flags().BoolVar(&opts.noBuild, "no-build", false,
		"Do not build new images before deploying services.")
	cmd.Flags().BoolVar(&opts.BuildServicesOptions.NoCache, "no-cache", false,
		"Do not use cache when building images.")
	cmd.Flags().StringSliceVarP(&opts.profiles, "profile", "p", nil,
		"One or more Compose profiles to enable.")
	cmd.Flags().BoolVar(&opts.recreate, "recreate", false,
		"Recreate containers even if their configuration and image haven't changed.")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false,
		"Auto-confirm deployment plan. Should be explicitly set when running non-interactively,\n"+
			"e.g., in CI/CD pipelines. [$UNCLOUD_AUTO_CONFIRM]")

	// TODO: Consider adding a filter flag to specify which machines to deploy to but keep the rest running.
	//  Could be useful to test a new version on a subset of machines before rolling out to all.

	return cmd
}

// runDeploy parses the Compose file(s) and deploys the services.
func runDeploy(ctx context.Context, uncli *cli.CLI, opts deployOptions) error {
	project, err := compose.LoadProject(ctx, opts.files, composecli.WithDefaultProfiles(opts.profiles...))
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

	servicesToBuild, err := cli.ServicesThatNeedBuild(project, opts.services, false)
	if err != nil {
		return fmt.Errorf("determine services to build: %w", err)
	}

	if len(servicesToBuild) > 0 {
		if opts.noBuild {
			fmt.Println("Not building services as requested.")
		} else {
			// Build service images without pushing them to cluster yet to not connect to the cluster twice.
			opts.BuildServicesOptions.Deps = true // build dependencies as deploy includes them by default
			opts.BuildServicesOptions.Services = opts.services

			if err = uncli.BuildServices(ctx, project, opts.BuildServicesOptions); err != nil {
				return fmt.Errorf("build services: %w", err)
			}
		}
		fmt.Println()
	}

	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	if len(servicesToBuild) > 0 && !opts.noBuild {
		// Push built service images to cluster machines one at a time.
		var errs []error
		for _, s := range servicesToBuild {
			if s.Image == "" {
				// Skip services without an image name (shouldn't happen for services with build config).
				continue
			}

			// Push to the specified x-machines or to *all* cluster machines if not specified.
			var pushOpts client.PushImageOptions
			if machines, ok := s.Extensions[compose.MachinesExtensionKey].(compose.MachinesSource); ok {
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
			}, uncli.ProgressOut(), fmt.Sprintf("Pushing image %s to cluster", boldStyle.Render(s.Image)))
			// Collect errors to try pushing all images.
			if err != nil {
				errs = append(errs, err)
			}
		}

		if err = errors.Join(errs...); err != nil {
			return err
		}
		fmt.Println()
	}

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

	fmt.Println(lipgloss.NewStyle().Bold(true).Render("Deployment plan"))
	if err = printPlan(ctx, clusterClient, plan); err != nil {
		return fmt.Errorf("print deployment plan: %w", err)
	}
	fmt.Println()

	// Ask for plan confirmation before proceeding with the deployment unless auto-confirmed with --yes.
	if !opts.yes {
		if !cli.IsStdinTerminal() {
			return errors.New("cannot ask to confirm deployment plan in non-interactive mode, " +
				"use --yes flag or set UNCLOUD_AUTO_CONFIRM=true to auto-confirm")
		}

		confirmed, err := cli.Confirm()
		if err != nil {
			return fmt.Errorf("confirm deployment: %w", err)
		}
		if !confirmed {
			fmt.Println("Cancelled. No changes were made.")
			return nil
		}
	}

	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		if err := plan.Execute(ctx, clusterClient); err != nil {
			return fmt.Errorf("deploy services: %w", err)
		}
		return nil
	}, uncli.ProgressOut(), "Deploying services")
}

func printPlan(ctx context.Context, cli *client.Client, plan operation.SequenceOperation) error {
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
