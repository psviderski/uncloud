package image

import (
	"context"
	"errors"
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/spf13/cobra"
)

type removeOptions struct {
	machines []string
	yes      bool
}

func NewRemoveCommand() *cobra.Command {
	opts := removeOptions{}
	cmd := &cobra.Command{
		Use:     "rm IMAGE [IMAGE...]",
		Aliases: []string{"remove", "delete"},
		Short:   "Remove images from machines in the cluster.",
		Long: `Remove images from all or selected machines in the cluster.

An image name without a tag selects all its tags. Specify a tag, such as myapp:latest, to remove
only that tag.

Wildcard patterns are supported for image names and tags and should be quoted to prevent your
shell from expanding them. Removing by image ID attempts to remove each name for that image
independently.

Images used by running or stopped containers cannot be removed. Unused tags can be removed even
when the same image is used under another tag. Failed removals do not prevent other removals.`,
		Example: `  # Remove a tag from all machines after confirming the plan.
  uc image rm myapp:old

  # Remove all tags for an image name from all machines.
  uc image rm myapp

  # Remove matching tags from two machines without confirmation.
  uc image rm 'myapp:1.*' 'otherapp:*' -m machine1,machine2 --yes

  # Remove all names for an image by ID from all machines.
  uc image rm ee8fed0e9618`,
		Args: cobra.MinimumNArgs(1),
		PreRun: func(cmd *cobra.Command, args []string) {
			cli.BindEnvToFlag(cmd, "yes", "UNCLOUD_AUTO_CONFIRM")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return remove(cmd.Context(), uncli, args, opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Machine names or IDs to remove images from. Can be specified multiple times or as a comma-separated list.\n"+
			"If not specified, the matching images will be removed from all machines.")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false,
		"Auto-confirm the removal plan. Should be explicitly set when running non-interactively,\n"+
			"e.g., in CI/CD pipelines. [$UNCLOUD_AUTO_CONFIRM]")

	completion.MachinesFlag(cmd)

	return cmd
}

func remove(ctx context.Context, uncli *cli.CLI, images []string, opts removeOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	plan, err := planImageRemoval(ctx, clusterClient, images, cli.ExpandCommaSeparatedValues(opts.machines))
	if err != nil {
		return fmt.Errorf("plan image removal: %w", err)
	}
	directConn := uncli.DirectConnection()
	contextName := uncli.ContextOverrideOrCurrent()
	cluster := ""
	if directConn != "" {
		cluster = directConn
		lipgloss.Println(tui.Faint.Render("connection: ") + tui.NameStyle.Render(directConn))
		lipgloss.Println()
	} else if contextName != "" && len(uncli.Config.Contexts) > 1 {
		// Only show context if there's more than one to avoid unnecessary clutter.
		cluster = contextName
		lipgloss.Println(tui.Faint.Render("context: ") + tui.NameStyle.Render(contextName))
		lipgloss.Println()
	}

	lipgloss.Println(plan.Format())

	if !opts.yes {
		if !tui.IsTerminalAvailable() {
			return errors.New("cannot ask to confirm image removal in non-interactive mode, " +
				"use --yes flag or set UNCLOUD_AUTO_CONFIRM=true to auto-confirm")
		}

		title := "Proceed with image removal?"
		if cluster != "" {
			isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
			confirmStyle := tui.ThemeConfirm().Theme(isDark).Focused.Title
			title = "Proceed with image removal on " + tui.NameStyle.Render(cluster) + confirmStyle.Render("?")
		}

		confirmed, err := tui.Confirm(title)
		if err != nil {
			return fmt.Errorf("confirm image removal: %w", err)
		}
		if !confirmed {
			return cli.Cancelled("Image removal cancelled. No images were removed.")
		}
	}

	untagged := make(map[string]bool)
	var errs []error
	for i, group := range plan.GroupPlans {
		if i > 0 {
			fmt.Println()
		}

		err := progress.RunWithTitle(ctx, func(ctx context.Context) error {
			return group.Execute(ctx, clusterClient, untagged)
		}, uncli.ProgressOut(), "Removing image "+group.Image)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
