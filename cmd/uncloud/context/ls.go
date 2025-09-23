package context

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"text/tabwriter"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func NewListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List available cluster contexts.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(uncli)
		},
	}

	return cmd
}

func list(uncli *cli.CLI) error {
	if uncli.Config == nil {
		return fmt.Errorf("context management is not available: Uncloud configuration file is not being used")
	}

	if len(uncli.Config.Contexts) == 0 {
		fmt.Println("No contexts found")
		return nil
	}

	contextNames := slices.Sorted(maps.Keys(uncli.Config.Contexts))
	currentContext := uncli.Config.CurrentContext

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAME\tCURRENT\tCONNECTIONS")

	for _, name := range contextNames {
		current := ""
		if name == currentContext {
			current = "✓"
		}
		connCount := len(uncli.Config.Contexts[name].Connections)
		fmt.Fprintf(tw, "%s\t%s\t%d\n", name, current, connCount)
	}

	return tw.Flush()
}
