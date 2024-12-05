package service

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage services in an Uncloud cluster.",
	}
	cmd.AddCommand(
		NewListCommand(),
		NewRunCommand(),
	)
	return cmd
}
