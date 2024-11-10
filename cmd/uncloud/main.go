package main

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"uncloud/cmd/uncloud/machine"
	"uncloud/cmd/uncloud/service"
	"uncloud/internal/cli"
)

func main() {
	var configPath string
	cmd := &cobra.Command{
		Use:           "uncloud",
		Short:         "A CLI tool for managing Uncloud resources such as clusters, machines, and services.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if strings.HasPrefix(configPath, "~/") {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("get user home directory to resolve %q: %w", configPath, err)
				}
				configPath = strings.Replace(configPath, "~", home, 1)
			}

			uncli, err := cli.New(configPath)
			if err != nil {
				return fmt.Errorf("initialize CLI: %w", err)
			}
			cmd.SetContext(context.WithValue(cmd.Context(), "cli", uncli))
			return nil
		},
	}
	// TODO: allow to override using UNCLOUD_CONFIG env var.
	cmd.PersistentFlags().StringVar(&configPath, "uncloud-config", "~/.config/uncloud/config.toml",
		"path to the Uncloud configuration file.")
	_ = cmd.MarkPersistentFlagFilename("uncloud-config", "toml")

	cmd.AddCommand(
		machine.NewRootCommand(),
		service.NewRootCommand(),
		service.NewRunCommand(),
	)
	cobra.CheckErr(cmd.Execute())
}
