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
	"github.com/spf13/cobra"
)

type destroyOptions struct {
	cli.BuildServicesOptions

	files    []string
	profiles []string
	services []string
	yes      bool
	signal   string
	timeout  int
}

// NewDestroyCommand creates a new command to tear down services from a Compose file.
func NewDestroyCommand() *cobra.Command {
	opts := destroyOptions{}
	cmd := &cobra.Command{
		Use:     "destroy [FLAGS] [SERVICE...]",
		Aliases: []string{"down", "undeploy"},
		Short:   "Destroy services from a Compose file.",
		Long: `Destroy services from a Compose file.

Destroy removes all containers of the specified service(s) across all machines in the cluster.

See "uc service remove".`,
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
		"One or more Compose files to deploy services from. (default compose.yaml)")
	cmd.Flags().StringSliceVarP(&opts.profiles, "profile", "p", nil,
		"One or more Compose profiles to enable.")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false,
		"Auto-confirm deployment plan. Should be explicitly set when running non-interactively,\n"+
			"e.g., in CI/CD pipelines. [$UNCLOUD_AUTO_CONFIRM]")

	return cmd
}

// runDestroy parses the Compose file(s) and detroys the services.
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

	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	services := []api.Service{}
	for _, service := range composeServices {
		svc, err := client.InspectService(ctx, service)
		if err != nil {
			return fmt.Errorf("inspect service: %w", err)
		}
		services = append(services, svc)
	}

	t := tui.NewTable()
	headers := []string{"NAME", "MODE", "REPLICAS"}
	t.Headers(headers...)

	for _, s := range services {
		row := []string{s.Name, s.Mode, fmt.Sprintf("%d", len(s.Containers))}
		t.Row(row...)
	}
	fmt.Println(t)

	if !opts.yes {
		if !tui.IsStdinTerminal() {
			return errors.New("cannot ask to confirm undeployment in non-interactive mode, " +
				"use --yes flag or set UNCLOUD_AUTO_CONFIRM=true to auto-confirm")
		}

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

		title := "Proceed with destroy?"
		// Include the direct connection or context name in the confirmation prompt to avoid accidentally
		// deploying to the wrong cluster.
		if deployTarget != "" {
			isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
			confirmStyle := tui.ThemeConfirm().Theme(isDark).Focused.Title
			title = "Proceed with detroy to " + tui.NameStyle.Render(deployTarget) + confirmStyle.Render("?")
		}

		fmt.Println()
		confirmed, err := tui.Confirm(title)
		if err != nil {
			return fmt.Errorf("confirm destroy: %w", err)
		}
		if !confirmed {
			return cli.Cancelled("Destroy cancelled. No changes were made.")
		}
	}

	if opts.yes {
		fmt.Println() // slightly nicer in the output
	}
	for _, s := range composeServices {
		err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
			if err = client.RemoveService(ctx, s); err != nil {
				return fmt.Errorf("destroying service '%s': %w", s, err)
			}
			return nil
		}, uncli.ProgressOut(), "Destroy service "+s)
	}

	return err
}
