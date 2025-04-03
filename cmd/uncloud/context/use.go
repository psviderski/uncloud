package context

import (
	"fmt"
	"maps"
	"slices"

	"github.com/charmbracelet/huh"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func NewUseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use [CONTEXT]",
		Short: "Switch to a different cluster context.",
		Long: "Switch to a different cluster context. If no context is provided, " +
			"a list of available contexts will be displayed for selection.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			if len(args) == 1 {
				if err := uncli.SetCurrentContext(args[0]); err != nil {
					return fmt.Errorf("failed to set the current cluster context to '%s': %w", args[0], err)
				}
				fmt.Printf("Current cluster context is now '%s'.\n", args[0])
				return nil
			}

			return selectContext(uncli)
		},
	}

	return cmd
}

func selectContext(uncli *cli.CLI) error {
	if uncli.Config == nil {
		return fmt.Errorf("context management is not available: Uncloud configuration file is not being used")
	}
	if len(uncli.Config.Contexts) == 0 {
		return fmt.Errorf("no contexts found in Uncloud config (%s)", uncli.Config.Path())
	}

	contextNames := slices.Sorted(maps.Keys(uncli.Config.Contexts))

	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a cluster context").
				Options(buildContextOptions(contextNames, uncli.Config.CurrentContext)...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return fmt.Errorf("select cluster context: %w", err)
	}

	if err := uncli.SetCurrentContext(selected); err != nil {
		return fmt.Errorf("set current cluster context: %w", err)
	}

	fmt.Printf("Current cluster context is now '%s'.\n", selected)
	return nil
}

func buildContextOptions(contexts []string, current string) []huh.Option[string] {
	options := make([]huh.Option[string], len(contexts))

	for i, ctx := range contexts {
		opt := huh.NewOption(ctx, ctx)
		if ctx == current {
			opt.Key += " (current)"
			opt = opt.Selected(true)
		}

		options[i] = opt
	}

	options[1].Selected(true)
	return options
}
