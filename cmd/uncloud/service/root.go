package service

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service",
		Aliases: []string{"svc"},
		Short:   "Manage services in the cluster.",
	}
	cmd.AddCommand(
		NewExecCommand(""),
		NewInspectCommand(""),
		NewListCommand(""),
		NewLogsCommand(""),
		NewRmCommand(""),
		NewRunCommand(""),
		NewScaleCommand(""),
		NewStartCommand(""),
		NewStopCommand(""),
	)
	return cmd
}
