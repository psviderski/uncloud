package machine

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "machine",
		Short: "Manage machines in an Uncloud cluster.",
	}
	cmd.AddCommand(
		NewAddCommand(),
		NewInitCommand(),
	)
	return cmd
}
