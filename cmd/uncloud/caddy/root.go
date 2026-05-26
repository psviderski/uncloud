package caddy

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "caddy",
		Short: "Manage Caddy reverse proxy service.",
	}
	cmd.AddCommand(
		NewConfigCommand(),
		NewDeployCommand(),
		NewUpstreamsCommand(),
	)
	return cmd
}
