package main

import (
	"context"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
	"uncloud/cmd/ucind/cluster"
	"uncloud/internal/ucind"
)

func main() {
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

			p := ucind.NewProvisioner(cli)
			// Persist the provisioner in the context so it can be used by subcommands.
			cmd.SetContext(context.WithValue(cmd.Context(), "provisioner", p))
			return nil
		},
	}

	cmd.AddCommand(
		cluster.NewRootCommand(),
	)
	cobra.CheckErr(cmd.Execute())
}
