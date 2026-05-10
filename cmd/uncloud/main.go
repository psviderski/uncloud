package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/psviderski/uncloud/cmd/uncloud/caddy"
	cmdcontext "github.com/psviderski/uncloud/cmd/uncloud/context"
	"github.com/psviderski/uncloud/cmd/uncloud/dns"
	"github.com/psviderski/uncloud/cmd/uncloud/image"
	cmdmachine "github.com/psviderski/uncloud/cmd/uncloud/machine"
	"github.com/psviderski/uncloud/cmd/uncloud/service"
	"github.com/psviderski/uncloud/cmd/uncloud/volume"
	"github.com/psviderski/uncloud/cmd/uncloud/wg"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/psviderski/uncloud/internal/fs"
	"github.com/psviderski/uncloud/internal/log"
	"github.com/psviderski/uncloud/internal/machine"
	"github.com/psviderski/uncloud/internal/version"
	"github.com/spf13/cobra"
)

type globalOptions struct {
	configPath string
	connect    string
	context    string
}

func main() {
	log.InitLoggerFromEnv()

	opts := globalOptions{}
	cmd := &cobra.Command{
		Use:           "uc",
		Short:         "A CLI tool for managing Uncloud resources such as machines, services, and volumes.",
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cli.BindEnvToFlag(cmd, "connect", "UNCLOUD_CONNECT")
			cli.BindEnvToFlag(cmd, "context", "UNCLOUD_CONTEXT")
			cli.BindEnvToFlag(cmd, "uncloud-config", "UNCLOUD_CONFIG")

			var conn *config.MachineConnection
			if opts.connect != "" {
				if after, ok := strings.CutPrefix(opts.connect, "tcp://"); ok {
					addrPort, err := netip.ParseAddrPort(after)
					if err != nil {
						return fmt.Errorf("parse TCP address: %w", err)
					}
					conn = &config.MachineConnection{
						TCP: &addrPort,
					}
				} else if after, ok := strings.CutPrefix(opts.connect, "ssh+go://"); ok {
					conn = &config.MachineConnection{
						SSHGo: config.SSHDestination(after),
					}
				} else if after, ok := strings.CutPrefix(opts.connect, "ssh+cli://"); ok {
					// Backward-compatible alias for ssh://.
					conn = &config.MachineConnection{
						SSH: config.SSHDestination(after),
					}
				} else if strings.HasPrefix(opts.connect, "unix://") {
					conn = &config.MachineConnection{
						Unix: opts.connect[len("unix://"):],
					}
				} else {
					// Default: system ssh CLI command (no prefix or ssh:// prefix).
					dest := strings.TrimPrefix(opts.connect, "ssh://")
					conn = &config.MachineConnection{
						SSH: config.SSHDestination(dest),
					}
				}
			}

			configPath := fs.ExpandHomeDir(opts.configPath)

			if opts.connect == "" {
				if !fs.Exists(configPath) && fs.Exists(machine.DefaultUncloudSockPath) {
					conn = &config.MachineConnection{
						Unix: machine.DefaultUncloudSockPath,
					}
				}
			}

			uncli, err := cli.New(configPath, conn, opts.context)
			if err != nil {
				return fmt.Errorf("initialise CLI: %w", err)
			}
			cmd.SetContext(context.WithValue(cmd.Context(), "cli", uncli))
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&opts.connect, "connect", "",
		"Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]\n"+
			"Format: [ssh://]user@host[:port], ssh+go://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock")
	cmd.PersistentFlags().StringVar(&opts.configPath, "uncloud-config", "~/.config/uncloud/config.yaml",
		"Path to the Uncloud configuration file. [$UNCLOUD_CONFIG]")
	_ = cmd.MarkPersistentFlagFilename("uncloud-config", "yaml", "yml")
	cmd.PersistentFlags().StringVarP(&opts.context, "context", "c", "",
		"Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]")

	// Set custom help function to show links to docs and Discord only for the root 'uc' command.
	defaultHelpFunc := cmd.HelpFunc()
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		defaultHelpFunc(c, args)
		// Only show links for the root 'uc' command.
		if c.Name() == "uc" {
			urlStyle := lipgloss.NewStyle().
				Underline(true).
				Foreground(lipgloss.Color("12")) // light blue

			fmt.Fprintln(c.OutOrStdout())
			fmt.Fprintf(c.OutOrStdout(), "Learn more about Uncloud:       %s\n",
				urlStyle.Render("https://uncloud.run/docs"))
			fmt.Fprintf(c.OutOrStdout(), "Join our Discord community:     %s\n",
				urlStyle.Render("https://uncloud.run/discord"))
		}
	})

	cmd.AddGroup(&cobra.Group{
		ID:    "service",
		Title: "Deploy and manage services:",
	})

	cmd.AddCommand(
		NewBuildCommand(),
		NewDeployCommand(),
		NewDocsCommand(),
		NewImagesCommand(),
		NewPsCommand(),
		caddy.NewRootCommand(),
		cmdcontext.NewRootCommand(),
		dns.NewRootCommand(),
		image.NewRootCommand(),
		cmdmachine.NewRootCommand(),
		service.NewRootCommand(),
		service.NewExecCommand("service"),
		service.NewInspectCommand("service"),
		service.NewListCommand("service"),
		service.NewLogsCommand("service"),
		service.NewRmCommand("service"),
		service.NewRunCommand("service"),
		service.NewScaleCommand("service"),
		service.NewStartCommand("service"),
		service.NewStopCommand("service"),
		volume.NewRootCommand(),
		wg.NewRootCommand(),
	)
	if err := cmd.Execute(); err != nil {
		if cancelled, ok := errors.AsType[*cli.CancelledError](err); ok {
			fmt.Fprintln(os.Stderr, cancelled.Error())
			os.Exit(1)
		}
		cobra.CheckErr(err)
	}
}
