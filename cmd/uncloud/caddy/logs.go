package caddy

import (
	"github.com/psviderski/uncloud/cmd/uncloud/service"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/logs"
	"github.com/spf13/cobra"
)

func NewLogsCommand() *cobra.Command {
	var options logs.Options

	cmd := &cobra.Command{
		Use:     "logs",
		Aliases: []string{"log"},
		Short:   "View caddy logs.",
		Long: `View caddy logs.

This calls "uc logs caddy", see "uc logs" for the documention.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			args = append([]string{"caddy"}, args...)
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return service.RunLogs(cmd.Context(), uncli, args, options)
		},
	}
	cmd.Flags().AddFlagSet(logs.Flags(&options))
	return cmd
}
