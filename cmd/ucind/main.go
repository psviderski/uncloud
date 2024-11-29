package main

import (
	"context"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"uncloud/cmd/ucind/cluster"
	"uncloud/internal/ucind"
)

func main() {
	var configPath string
	cmd := &cobra.Command{
		Use:   "ucind",
		Short: "A CLI tool for running Uncloud test clusters using Docker.",
		Long: "A CLI tool for running Uncloud test clusters using Docker.\n" +
			"Machines in a ucind cluster are Docker containers running a ucind image. All machines within a cluster " +
			"are connected to the same Docker network.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
			if err != nil {
				return fmt.Errorf("create Docker client: %w", err)
			}

			if strings.HasPrefix(configPath, "~/") {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("get user home directory to resolve %q: %w", configPath, err)
				}
				configPath = strings.Replace(configPath, "~", home, 1)
			}
			configUpdater := ucind.NewConfigUpdater(configPath)

			p := ucind.NewProvisioner(cli, configUpdater)
			// Persist the provisioner in the context so it can be used by subcommands.
			cmd.SetContext(context.WithValue(cmd.Context(), "provisioner", p))
			return nil
		},
	}

	// TODO: allow to override using UNCLOUD_CONFIG env var.
	cmd.PersistentFlags().StringVar(&configPath, "uncloud-config", "~/.config/uncloud/config.toml",
		"path to the Uncloud configuration file.")
	_ = cmd.MarkPersistentFlagFilename("uncloud-config", "toml")

	cmd.AddCommand(
		cluster.NewRootCommand(),
	)
	cobra.CheckErr(cmd.Execute())
}
