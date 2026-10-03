package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/spf13/cobra"
)

type destroyOptions struct {
	cli.BuildServicesOptions

	files    []string
	profiles []string
	services []string
	yes      bool
}

// NewDestroyCommand creates a new command to tear down services from a Compose file.
func NewDestroyCommand() *cobra.Command {
	opts := destroyOptions{}
	cmd := &cobra.Command{
		Use:     "destroy [FLAGS] [SERVICE...]",
		Aliases: []string{"down"},
		Short:   "Destroy services from a Compose file.",
		Long: `Destroy services from a Compose file.

Destroy removes all containers of the specified service(s) across all machines in the cluster.

See "uc service remove" for more details.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.BindEnvToFlag(cmd, "yes", "UNCLOUD_AUTO_CONFIRM")

			uncli := cmd.Context().Value("cli").(*cli.CLI)
			opts.services = args

			return runDestroy(cmd.Context(), uncli, opts)
		},
		GroupID: "service",
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return completion.ComposeServices(cmd.Context(), args, toComplete, opts.files, opts.profiles)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.files, "file", "f", nil,
		"One or more Compose files to destroy services from. (default compose.yaml)")
	cmd.Flags().StringSliceVarP(&opts.profiles, "profile", "p", nil,
		"One or more Compose profiles to enable.")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false,
		"Auto-confirm deployment plan. Should be explicitly set when running non-interactively,\n"+
			"e.g., in CI/CD pipelines. [$UNCLOUD_AUTO_CONFIRM]")

	return cmd
}

// runDestroy parses the Compose file(s) and destroys the services.
func runDestroy(ctx context.Context, uncli *cli.CLI, opts destroyOptions) error {
	project, err := compose.LoadProject(ctx, opts.files, composecli.WithDefaultProfiles(opts.profiles...))
	if err != nil {
		return fmt.Errorf("load compose file(s): %w", err)
	}

	uncli.SetClusterContextIfUnset(compose.ClusterContext(project))

	if len(opts.services) > 0 {
		project, err = project.WithSelectedServices(opts.services)
		if err != nil {
			return fmt.Errorf("select services: %w", err)
		}
	}

	composeServices := append(project.ServiceNames(), project.DisabledServiceNames()...)
	if len(composeServices) == 0 {
		return errors.New("no services found in Compose file(s)")
	}

	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	services := []api.Service{}
	for _, service := range composeServices {
		svc, err := clusterClient.InspectService(ctx, service)
		if err != nil {
			return fmt.Errorf("inspect service: %w", err)
		}
		services = append(services, svc)
	}

	strategy := &deploy.RemoveStrategy{}
	composeDestroy, err := compose.NewDeploymentWithStrategy(ctx, clusterClient, project, strategy)
	if err != nil {
		return fmt.Errorf("create compose destruction: %w", err)
	}

	plan, err := composeDestroy.Plan(ctx)
	if err != nil {
		return fmt.Errorf("plan destruction: %w", err)
	}

	if plan.IsEmpty() {
		fmt.Println("Services already removed.")
		return nil
	}

	fmt.Println(tui.Bold.Underline(true).Render("Destruction plan"))
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
		if !tui.IsTerminalAvailable() {
			return errors.New("cannot ask to confirm deployment plan in non-interactive mode, " +
				"use --yes flag or set UNCLOUD_AUTO_CONFIRM=true to auto-confirm")
		}

		title := "Proceed with destruction?"
		// Include the direct connection or context name in the confirmation prompt to avoid accidentally
		// deploying to the wrong cluster.
		if deployTarget != "" {
			isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
			confirmStyle := tui.ThemeConfirm().Theme(isDark).Focused.Title
			title = "Proceed with destruction in " + tui.NameStyle.Render(deployTarget) + confirmStyle.Render("?")
		}

		confirmed, err := tui.Confirm(title)
		if err != nil {
			return fmt.Errorf("confirm destruction: %w", err)
		}
		if !confirmed {
			return cli.Cancelled("Destruction cancelled. No changes were made.")
		}
	}

	title := "Destroying"
	if deployTarget != "" {
		title += " in " + tui.NameStyle.Render(deployTarget)
	}
	err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
		if err := plan.Execute(ctx, clusterClient); err != nil {
			return fmt.Errorf("destroying services: %w", err)
		}
		return nil
	}, uncli.ProgressOut(), title)
	if err != nil {
		fmt.Println()
		return err
	}
	return nil
}
