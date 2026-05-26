package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"

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
	"github.com/psviderski/uncloud/internal/cli/tui"
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
	var initErr error
	cmd := &cobra.Command{
		Use:           "uc",
		Short:         "A CLI tool for managing Uncloud resources such as machines, services, and volumes.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if initErr != nil {
				return initErr
			}
			return initCLI(cmd, opts)
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

	cobra.OnInitialize(func() {
		initErr = initCLI(cmd, opts)
	})

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

func initCLI(cmd *cobra.Command, opts globalOptions) error {
	if cmd.Context().Value("cli") != nil {
		return nil
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
	setCLIContext(cmd.Root(), uncli)
	return nil
}

func setCLIContext(cmd *cobra.Command, uncli *cli.CLI) {
	cmd.SetContext(context.WithValue(cmd.Context(), "cli", uncli))
	for _, child := range cmd.Commands() {
		setCLIContext(child, uncli)
	}
}
