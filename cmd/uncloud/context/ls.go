package context

import (
	"fmt"
	"maps"
	"slices"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/output"
	"github.com/spf13/cobra"
)

type listOptions struct {
	format string
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
	cmd.Flags().StringVar(&opts.format, "format", "table", "Output format (table, json)")
	return cmd
}

type contextItem struct {
	Name        string `json:"name"`
	Current     bool   `json:"current"`
	Connections int    `json:"connections"`
}

func list(uncli *cli.CLI, opts listOptions) error {
	if uncli.Config == nil {
		return fmt.Errorf("context management is not available: Uncloud configuration file is not being used")
	}

	if len(uncli.Config.Contexts) == 0 {
		if opts.format == "json" {
			fmt.Println("[]")
			return nil
		}
		fmt.Println("No contexts found")
		return nil
	}

	contextNames := slices.Sorted(maps.Keys(uncli.Config.Contexts))
	currentContext := uncli.Config.CurrentContext

	var items []contextItem
	for _, name := range contextNames {
		items = append(items, contextItem{
			Name:        name,
			Current:     name == currentContext,
			Connections: len(uncli.Config.Contexts[name].Connections),
		})
	}

	columns := []output.Column[contextItem]{
		{Header: "NAME", Field: "Name"},
		{
			Header: "CURRENT",
			Accessor: func(item contextItem) string {
				if item.Current {
					return "✓"
				}
				return ""
			},
		},
		{Header: "CONNECTIONS", Field: "Connections"},
	}

	return output.Print(items, columns, opts.format)
}
