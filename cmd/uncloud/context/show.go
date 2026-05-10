package context

import (
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func NewShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show current cluster context.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return show(uncli)
		},
	}

	return cmd
}

func show(uncli *cli.CLI) error {
	// discard errors, only show the current context, otherwise nothing
	if uncli.Config == nil {
		return nil
	}

	if len(uncli.Config.Contexts) == 0 {
		return nil
	}
	fmt.Println(uncli.Config.CurrentContext)

	return nil
}
