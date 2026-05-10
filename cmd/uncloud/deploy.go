package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/pkg/stringid"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/logs"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/psviderski/uncloud/pkg/client/deploy/operation"
	"github.com/spf13/cobra"
)

type deployOptions struct {
	cli.BuildServicesOptions

	files      []string
	profiles   []string
	services   []string
	noBuild    bool
	recreate   bool
	skipHealth bool
	yes        bool
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
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return completion.ComposeServices(cmd.Context(), args, toComplete, opts.files, opts.profiles)
		},
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
	cmd.Flags().BoolVar(&opts.skipHealth, "skip-health", false,
		"Skip the monitoring period and health checks after starting new containers. Useful for faster emergency "+
			"deployments.\n"+
			"Warning: This may cause downtime if new containers fail to start properly.")
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

	uncli.SetClusterContextIfUnset(compose.ClusterContext(project))

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

	strategy := &deploy.RollingStrategy{
		ForceRecreate:     opts.recreate,
		SkipHealthMonitor: opts.skipHealth,
	}
	composeDeploy, err := compose.NewDeploymentWithStrategy(ctx, clusterClient, project, strategy)
	if err != nil {
		return fmt.Errorf("create compose deployment: %w", err)
	}

	plan, err := composeDeploy.Plan(ctx)
	if err != nil {
		return fmt.Errorf("plan deployment: %w", err)
	}

	if plan.IsEmpty() {
		fmt.Println("Services are up to date.")
		return nil
	}

	fmt.Println(tui.Bold.Underline(true).Render("Deployment plan"))
	fmt.Println()

	directConn := uncli.DirectConnection()
	contextName := uncli.ContextOverrideOrCurrent()
	deployTarget := ""
	if directConn != "" {
		deployTarget = directConn
		fmt.Println(tui.Faint.Render("connection: ") + tui.NameStyle.Render(directConn))
		fmt.Println()
	} else if contextName != "" && len(uncli.Config.Contexts) > 1 {
		// Only show context if there's more than one to avoid unnecessary clutter.
		deployTarget = contextName
		fmt.Println(tui.Faint.Render("context: ") + tui.NameStyle.Render(contextName))
		fmt.Println()
	}

	fmt.Println(plan.Format())

	// Ask for plan confirmation before proceeding with the deployment unless auto-confirmed with --yes.
	if !opts.yes {
		if !tui.IsStdinTerminal() {
			return errors.New("cannot ask to confirm deployment plan in non-interactive mode, " +
				"use --yes flag or set UNCLOUD_AUTO_CONFIRM=true to auto-confirm")
		}

		title := "Proceed with deployment?"
		// Include the direct connection or context name in the confirmation prompt to avoid accidentally
		// deploying to the wrong cluster.
		if deployTarget != "" {
			isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
			confirmStyle := tui.ThemeConfirm().Theme(isDark).Focused.Title
			title = "Proceed with deployment to " + tui.NameStyle.Render(deployTarget) + confirmStyle.Render("?")
		}

		confirmed, err := tui.Confirm(title)
		if err != nil {
			return fmt.Errorf("confirm deployment: %w", err)
		}
		if !confirmed {
			return cli.Cancelled("Deploy cancelled. No changes were made.")
		}
	}

	title := "Deploying"
	if deployTarget != "" {
		title += " to " + tui.NameStyle.Render(deployTarget)
	}
	err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
		if err := plan.Execute(ctx, clusterClient); err != nil {
			return fmt.Errorf("deploy services: %w", err)
		}
		return nil
	}, uncli.ProgressOut(), title)
	if err != nil {
		fmt.Println()

		tail := failedContainerLogsTail()
		if hookErr, ok := errors.AsType[*operation.PreDeployHookError](err); ok {
			printFailedContainerLogs(ctx, clusterClient,
				hookErr.ServiceName, hookErr.ContainerID, hookErr.MachineName, tail,
				fmt.Sprintf("Last %d log lines from failed pre-deploy hook:", tail))
			fmt.Println()
		} else if startErr, ok := errors.AsType[*operation.ContainerHealthError](err); ok {
			printFailedContainerLogs(ctx, clusterClient,
				startErr.ServiceName, startErr.ContainerID, startErr.MachineName, tail,
				fmt.Sprintf("Last %d log lines from failed container:", tail))
			fmt.Println()
		}

		return err
	}
	return nil
}

// printFailedContainerLogs fetches the last tail log lines from a container that failed during deployment and prints
// them using the standard log formatter under the provided header.
func printFailedContainerLogs(
	ctx context.Context, cli *client.Client, serviceName, containerID, machineName string, tail int, header string,
) {
	_, ch, err := cli.ServiceLogs(ctx, serviceName, api.ServiceLogsOptions{
		Containers: []string{containerID},
		Machines:   []string{machineName},
		Tail:       tail,
	})
	if err != nil {
		shortCtrID := stringid.TruncateID(containerID)
		fmt.Fprintf(os.Stderr, "Failed to fetch container logs '%s/%s': %v\n", serviceName, shortCtrID, err)
		fmt.Fprintf(os.Stderr, "You can try manually with: uc logs %s/%s\n", serviceName, shortCtrID)
		return
	}

	fmt.Println(tui.BoldRed.Render(header))

	logsEmpty := true
	formatter := logs.NewFormatter([]string{machineName}, []string{serviceName}, false)
	for entry := range ch {
		logsEmpty = false
		formatter.PrintEntry(entry)
	}

	if logsEmpty {
		fmt.Println("<no logs available>")
	}
}

// defaultFailedContainerLogsTail is the default number of recent log lines to print from a failed container to give
// the user immediate context without requiring a follow-up 'uc logs' invocation.
// Overridable via UNCLOUD_FAILED_CONTAINER_LOGS_TAIL.
const defaultFailedContainerLogsTail = 10

// failedContainerLogsTail returns the number of log lines to fetch from a failed container, honouring the
// UNCLOUD_FAILED_CONTAINER_LOGS_TAIL environment variable override when set and valid.
func failedContainerLogsTail() int {
	if v := os.Getenv("UNCLOUD_FAILED_CONTAINER_LOGS_TAIL"); v != "" {
		if tail, err := logs.Tail(v); err == nil && (tail == -1 || tail > 0) {
			return tail
		}
	}
	return defaultFailedContainerLogsTail
}
