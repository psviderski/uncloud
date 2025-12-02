package caddy

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

type configOptions struct {
	machine string
	noColor bool
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
	cmd.Flags().BoolVar(&opts.noColor, "no-color", false,
		"Disable syntax highlighting for the output.")

	return cmd
}

func runConfig(ctx context.Context, uncli *cli.CLI, opts configOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	if opts.machine != "" {
		// If a specific machine is requested, use it to get the Caddy configuration.
		ctx, _, err = clusterClient.ProxyMachinesContext(ctx, []string{opts.machine})
		if err != nil {
			return err
		}
	}

	config, err := clusterClient.Caddy.GetConfig(ctx, nil)
	if err != nil {
		return fmt.Errorf("get Caddy config: %w", err)
	}

	// Print the Caddyfile with syntax highlighting.
	if opts.noColor {
		fmt.Print(config.Caddyfile)
	} else {
		if err = quick.Highlight(os.Stdout, config.Caddyfile, "caddy", "terminal256", "monokai"); err != nil {
			// If highlighting fails, fall back to plain output.
			fmt.Print(config.Caddyfile)
		}
	}

	return nil
}
