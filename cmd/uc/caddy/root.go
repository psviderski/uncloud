package caddy

import (
	"github.com/psviderski/uncloud/cmd/uc/caddy/cert"
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "caddy",
		Short: "Manage Caddy reverse proxy service.",
	}
	cmd.AddCommand(
		cert.NewRootCommand(),
		NewConfigCommand(),
		NewDeployCommand(),
		NewLogsCommand(),
	)
	return cmd
}
