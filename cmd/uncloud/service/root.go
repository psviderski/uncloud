package service

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service",
		Aliases: []string{"svc"},
		Short:   "Manage services in an Uncloud cluster.",
	}
	cmd.AddCommand(
		NewInspectCommand(),
		NewListCommand(),
		NewRmCommand(),
		NewRunCommand(),
		NewScaleCommand(),
		NewExecCommand(),
	)
	return cmd
}
