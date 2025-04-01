package main

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/psviderski/uncloud/cmd/uncloud/caddy"
	"github.com/psviderski/uncloud/cmd/uncloud/dns"
	"github.com/psviderski/uncloud/cmd/uncloud/machine"
	"github.com/psviderski/uncloud/cmd/uncloud/service"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/psviderski/uncloud/internal/fs"
	"github.com/spf13/cobra"
)

type globalOptions struct {
	configPath string
	connect    string
}

func main() {
	opts := globalOptions{}
	cmd := &cobra.Command{
		Use:           "uncloud",
		Short:         "A CLI tool for managing Uncloud resources such as clusters, machines, and services.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var conn *config.MachineConnection
			if opts.connect != "" {
				if strings.HasPrefix(opts.connect, "tcp://") {
					addrPort, err := netip.ParseAddrPort(opts.connect[len("tcp://"):])
					if err != nil {
						return fmt.Errorf("parse TCP address: %w", err)
					}
					conn = &config.MachineConnection{
						TCP: addrPort,
					}
				} else {
					dest := opts.connect
					if strings.HasPrefix(dest, "ssh://") {
						dest = dest[len("ssh://"):]
					}
					conn = &config.MachineConnection{
						SSH: config.SSHDestination(dest),
					}
				}
			}

			configPath := fs.ExpandHomeDir(opts.configPath)
			uncli, err := cli.New(configPath, conn)
			if err != nil {
				return fmt.Errorf("initialise CLI: %w", err)
			}
			cmd.SetContext(context.WithValue(cmd.Context(), "cli", uncli))
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&opts.connect, "connect", "",
		"Connect to a remote cluster machine without using the Uncloud configuration file.\n"+
			"Format: [ssh://]user@host[:port] or tcp://host:port")
	// TODO: allow to override using UNCLOUD_CONFIG env var.
	cmd.PersistentFlags().StringVar(&opts.configPath, "uncloud-config", "~/.config/uncloud/config.yaml",
		"Path to the Uncloud configuration file.")
	_ = cmd.MarkPersistentFlagFilename("uncloud-config", "yaml", "yml")

	cmd.AddCommand(
		NewDeployCommand(),
		caddy.NewRootCommand(),
		dns.NewRootCommand(),
		machine.NewRootCommand(),
		service.NewRootCommand(),
		service.NewInspectCommand(),
		service.NewListCommand(),
		service.NewRmCommand(),
		service.NewRunCommand(),
		service.NewScaleCommand(),
	)
	cobra.CheckErr(cmd.Execute())
}
