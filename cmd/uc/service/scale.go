package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type scaleOptions struct {
	service  string
	replicas uint
	yes      bool
}

func NewScaleCommand(groupID string) *cobra.Command {
	opts := scaleOptions{}
	cmd := &cobra.Command{
		Use:   "scale SERVICE REPLICAS",
		Short: "Scale a replicated service by changing the number of replicas.",
		Long:  "Scale a replicated service by changing the number of replicas.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.BindEnvToFlag(cmd, "yes", "UNCLOUD_AUTO_CONFIRM")

			uncli := cmd.Context().Value("cli").(*cli.CLI)

			opts.service = args[0]
			replicas, err := strconv.ParseUint(args[1], 10, 0)
			if err != nil {
				return fmt.Errorf("invalid number of replicas: %w", err)
			}
			opts.replicas = uint(replicas)

			return scale(cmd.Context(), uncli, opts)
		},
		GroupID: groupID,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return completion.Services(cmd.Context(), uncli, args, toComplete)
		},
	}

	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false,
		"Auto-confirm scaling plan. Should be explicitly set when running non-interactively,\n"+
			"e.g., in CI/CD pipelines. [$UNCLOUD_AUTO_CONFIRM]")

	return cmd
}

func scale(ctx context.Context, uncli *cli.CLI, opts scaleOptions) error {
	if opts.replicas == 0 {
		return fmt.Errorf(
			"scaling to zero replicas is not supported. This would effectively remove the service without preserving "+
				"its configuration, making it impossible to scale back up. Uncloud derives the service configuration "+
				"from existing containers. Use 'uc rm %s' instead if you want to remove the service",
			opts.service,
		)
	}

	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	svc, err := clusterClient.InspectService(ctx, opts.service)
	if err != nil {
		return fmt.Errorf("inspect service '%s': %w", opts.service, err)
	}

	if svc.Mode != api.ServiceModeReplicated {
		return fmt.Errorf("scaling is only supported for services in %s mode, service '%s' is in %s mode",
			api.ServiceModeReplicated, svc.Name, svc.Mode)
	}

	currentReplicas := uint(len(svc.Containers))

	if currentReplicas == opts.replicas {
		fmt.Printf("Service %s already has %s replicas. No changes required.\n",
			tui.NameStyle.Render(svc.Name), tui.Bold.Render(fmt.Sprintf("%d", currentReplicas)))
		return nil
	}

	// TODO: Check if all containers have the same spec. If not, prompt user to choose which one to scale.
	//  This can happen if a service deployment failed midway and some containers were not updated.
	spec := svc.Containers[0].Container.ServiceSpec
	spec.Replicas = opts.replicas
	deployment := clusterClient.NewDeployment(spec, nil)
	plan, err := deployment.Plan(ctx)
	if err != nil {
		return fmt.Errorf("plan deployment: %w", err)
	}

	if len(plan.Operations) == 0 {
		fmt.Printf("Service %s is already scaled to %d replicas.\n", tui.NameStyle.Render(svc.Name), opts.replicas)
		return nil
	}

	fmt.Println(tui.Bold.Underline(true).Render("Scaling plan"))
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

	summary := plan.FormatSummary()
	fmt.Println(tui.Faint.Render(strings.Repeat("─", lipgloss.Width(summary))))
	fmt.Println(summary)
	fmt.Println()

	// Ask for confirmation unless auto-confirmed with --yes.
	if !opts.yes {
		if !tui.IsStdinTerminal() {
			return errors.New("cannot ask to confirm scaling plan in non-interactive mode, " +
				"use --yes flag or set UNCLOUD_AUTO_CONFIRM=true to auto-confirm")
		}

		title := "Proceed with scaling?"
		// Include the direct connection or context name in the confirmation prompt to avoid accidentally
		// scaling on the wrong cluster.
		if deployTarget != "" {
			isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
			confirmStyle := tui.ThemeConfirm().Theme(isDark).Focused.Title
			title = "Proceed with scaling on " + tui.NameStyle.Render(deployTarget) + confirmStyle.Render("?")
		}

		confirmed, err := tui.Confirm(title)
		if err != nil {
			return fmt.Errorf("confirm scaling: %w", err)
		}
		if !confirmed {
			return cli.Cancelled("Scaling cancelled. No changes were made.")
		}
	}

	title := fmt.Sprintf("Scaling service %s (%d → %d replicas)",
		tui.NameStyle.Render(svc.Name), currentReplicas, opts.replicas)
	err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
		if _, err = deployment.Run(ctx); err != nil {
			return fmt.Errorf("deploy service: %w", err)
		}
		return nil
	}, uncli.ProgressOut(), title)
	if err != nil {
		return err
	}

	return nil
}
