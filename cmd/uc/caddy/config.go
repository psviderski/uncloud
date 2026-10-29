package caddy

import (
	"context"
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2/quick"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
)

type configOptions struct {
	machine string
}

func NewConfigCommand() *cobra.Command {
	opts := configOptions{}

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show the current Caddy configuration (Caddyfile).",
		Long:  "Display the current Caddy configuration (Caddyfile) from the connected machine or a specified one.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return runConfig(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.machine, "machine", "m", "",
		"Name or ID of the machine to get the configuration from. (default is connected machine)")

	completion.MachinesFlag(cmd)

	return cmd
}

func runConfig(ctx context.Context, uncli *cli.CLI, opts configOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	config, err := clusterClient.Caddy.Config(ctx, client.CaddyConfigOptions{Machine: opts.machine})
	if err != nil {
		return fmt.Errorf("get Caddy config: %w", err)
	}

	// Print the Caddyfile with syntax highlighting.
	if err = quick.Highlight(lipgloss.Writer, config.Caddyfile, "caddy", "terminal256", "monokai"); err != nil {
		// If highlighting fails, fall back to plain output.
		lipgloss.Print(config.Caddyfile)
	}

	if config.LastReconciliationError != "" {
		tui.PrintWarning(fmt.Sprintf("last Caddy config load failed: %s\nShowing the last saved Caddyfile.",
			config.LastReconciliationError))
	}

	return nil
}
