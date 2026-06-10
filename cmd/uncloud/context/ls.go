package context

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/psviderski/uncloud/internal/cli/output"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/spf13/cobra"
)

type listOptions struct {
	output string
}

func NewListCommand() *cobra.Command {
	opts := listOptions{}

	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List available cluster contexts.",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(uncli, opts)
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return output.FlagValue(opts.output)
		},
	}

	output.Flag(cmd, &opts.output)

	return cmd
}

func list(uncli *cli.CLI, opts listOptions) error {
	if uncli.Config == nil {
		return fmt.Errorf("context management is not available: Uncloud configuration file is not being used")
	}

	if opts.output == "json" {
		type Contexts struct { // Wrap in Contexts type to create array of Contexts.
			Contexts map[string]*config.Context `json:"Contexts"`
		}
		data, _ := json.MarshalIndent(Contexts{uncli.Config.Contexts}, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(uncli.Config.Contexts) == 0 {
		fmt.Println("No contexts found")
		return nil
	}

	contextNames := slices.Sorted(maps.Keys(uncli.Config.Contexts))
	currentContext := uncli.Config.CurrentContext

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
