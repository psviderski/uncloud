package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"

	"github.com/psviderski/uncloud/cmd/uc/caddy"
	cmdcontext "github.com/psviderski/uncloud/cmd/uc/context"
	"github.com/psviderski/uncloud/cmd/uc/dns"
	"github.com/psviderski/uncloud/cmd/uc/image"
	cmdmachine "github.com/psviderski/uncloud/cmd/uc/machine"
	"github.com/psviderski/uncloud/cmd/uc/service"
	"github.com/psviderski/uncloud/cmd/uc/volume"
	"github.com/psviderski/uncloud/cmd/uc/wg"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/internal/fs"
	"github.com/psviderski/uncloud/internal/log"
	"github.com/psviderski/uncloud/internal/machine"
	"github.com/psviderski/uncloud/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Shell completion runs through the hidden __complete command which has flag parsing disabled,
			// so the global flags from the completed command line are never parsed. Apply them manually to make
			// completion work with --connect, --context, and --uncloud-config.
			if cmd.Name() == cobra.ShellCompRequestCmd {
				applyGlobalFlagsFromCompletionArgs(cmd.Root().PersistentFlags(), os.Args[1:])
			}

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
			fmt.Fprintln(c.OutOrStdout())
			fmt.Fprintf(c.OutOrStdout(), "Learn more about Uncloud:       %s\n",
				tui.URLStyle.Render(version.DocsURL))
			fmt.Fprintf(c.OutOrStdout(), "Join our Discord community:     %s\n",
				tui.URLStyle.Render(version.DiscordURL))
		}
	})

	cmd.AddGroup(&cobra.Group{
		ID:    "service",
		Title: "Deploy and manage services:",
	})

	cmd.AddCommand(
		NewBuildCommand(),
		NewDeployCommand(),
		NewDestroyCommand(),
		NewDocsCommand(),
		NewImagesCommand(),
		NewPsCommand(),
		NewProxyCommand(),
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
		NewVersionCommand(),
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

// applyGlobalFlagsFromCompletionArgs parses the global flags from the raw arguments of a __complete command and applies
// the ones found to flags. The trailing word being completed, unknown flags, and positional arguments are ignored.
func applyGlobalFlagsFromCompletionArgs(flags *pflag.FlagSet, args []string) {
	// The shell always passes the word being completed as the last argument, even if it's empty.
	// Exclude it from parsing as its value may not be complete yet.
	if len(args) == 0 {
		return
	}
	args = args[:len(args)-1]

	fset := pflag.NewFlagSet("global", pflag.ContinueOnError)
	fset.ParseErrorsAllowlist.UnknownFlags = true
	fset.String("connect", "", "")
	fset.StringP("context", "c", "", "")
	fset.String("uncloud-config", "", "")
	// Parsing an incomplete command line may fail, apply the flags parsed so far anyway.
	_ = fset.Parse(args)

	fset.Visit(func(f *pflag.Flag) {
		// Setting the flag marks it as changed so it takes precedence over environment variables.
		_ = flags.Set(f.Name, f.Value.String())
	})
}
