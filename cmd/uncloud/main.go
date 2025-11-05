package main

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/psviderski/uncloud/cmd/uncloud/caddy"
	cmdcontext "github.com/psviderski/uncloud/cmd/uncloud/context"
	"github.com/psviderski/uncloud/cmd/uncloud/dns"
	"github.com/psviderski/uncloud/cmd/uncloud/image"
	"github.com/psviderski/uncloud/cmd/uncloud/machine"
	"github.com/psviderski/uncloud/cmd/uncloud/service"
	"github.com/psviderski/uncloud/cmd/uncloud/volume"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/psviderski/uncloud/internal/fs"
	"github.com/psviderski/uncloud/internal/log"
	"github.com/psviderski/uncloud/internal/version"
	"github.com/spf13/cobra"
)

type globalOptions struct {
	configPath string
	connect    string
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
			cli.BindEnvToFlag(cmd, "uncloud-config", "UNCLOUD_CONFIG")

			var conn *config.MachineConnection
			if opts.connect != "" {
				if strings.HasPrefix(opts.connect, "tcp://") {
					addrPort, err := netip.ParseAddrPort(opts.connect[len("tcp://"):])
					if err != nil {
						return fmt.Errorf("parse TCP address: %w", err)
					}
					conn = &config.MachineConnection{
						TCP: &addrPort,
					}
				} else if strings.HasPrefix(opts.connect, "ssh+cli://") {
					dest := opts.connect[len("ssh+cli://"):]
					conn = &config.MachineConnection{
						SSHCLI: config.SSHDestination(dest),
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
		"Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]\n"+
			"Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], or tcp://host:port")
	cmd.PersistentFlags().StringVar(&opts.configPath, "uncloud-config", "~/.config/uncloud/config.yaml",
		"Path to the Uncloud configuration file. [$UNCLOUD_CONFIG]")
	_ = cmd.MarkPersistentFlagFilename("uncloud-config", "yaml", "yml")
	// TODO: make --context a global flag and pass it as a value of the command context.

	cmd.AddCommand(
		NewDeployCommand(),
		NewDocsCommand(),
		NewBuildCommand(),
		NewImagesCommand(),
		caddy.NewRootCommand(),
		cmdcontext.NewRootCommand(),
		dns.NewRootCommand(),
		image.NewRootCommand(),
		machine.NewRootCommand(),
		service.NewRootCommand(),
		service.NewExecCommand(),
		service.NewInspectCommand(),
		service.NewListCommand(),
		service.NewRmCommand(),
		service.NewRunCommand(),
		service.NewScaleCommand(),
		volume.NewRootCommand(),
	)
	cobra.CheckErr(cmd.Execute())
}
