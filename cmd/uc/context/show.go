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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return show(uncli)
		},
	}

	return cmd
}

func show(uncli *cli.CLI) error {
	fmt.Println(uncli.ContextOverrideOrCurrent())
	return nil
}
