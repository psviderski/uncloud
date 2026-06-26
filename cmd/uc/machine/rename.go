package machine

import (
	"context"
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func NewRenameCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename OLD_NAME NEW_NAME",
		Short: "Rename a machine in the cluster.",
		Long: `Rename a machine in the cluster.

This command changes the name of an existing machine while preserving all other
configuration including network settings, public IP, and cluster membership.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return rename(cmd.Context(), uncli, args[0], args[1])
		},
	}
	return cmd
}

func rename(ctx context.Context, uncli *cli.CLI, oldName, newName string) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	machine, err := client.RenameMachine(ctx, oldName, newName)
	if err != nil {
		return fmt.Errorf("rename machine: %w", err)
	}

	fmt.Printf("Machine %q renamed to %q (ID: %s)\n", oldName, machine.Name, machine.Id)
	return nil
}
