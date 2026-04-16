package context

import (
	"fmt"
	"maps"
	"slices"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/spf13/cobra"
)

type listOptions struct {
	short bool
}

func NewListCommand() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List available cluster contexts.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(uncli, opts)
		},
	}

	cmd.Flags().BoolVar(&opts.short, "short", false,
		"Show the current cluster context only.")

	return cmd
}

func list(uncli *cli.CLI, opts listOptions) error {
	if uncli.Config == nil {
		return fmt.Errorf("context management is not available: Uncloud configuration file is not being used")
	}

	if len(uncli.Config.Contexts) == 0 {
		if !opts.short {
			fmt.Println("No contexts found")
		}
		return nil
	}

	contextNames := slices.Sorted(maps.Keys(uncli.Config.Contexts))
	currentContext := uncli.Config.CurrentContext

	if opts.short {
		for _, name := range contextNames {
			if name == currentContext {
				fmt.Println(name)
				break
			}
		}
		return nil
	}

	t := tui.NewTable()
	t.Headers("NAME", "CURRENT", "CONNECTIONS")

	for _, name := range contextNames {
		current := ""
		if name == currentContext {
			current = "✓"
		}
		connCount := len(uncli.Config.Contexts[name].Connections)
		t.Row(name, current, fmt.Sprintf("%d", connCount))
	}

	fmt.Println(t)
	return nil
}
