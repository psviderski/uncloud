package machine

import (
	"context"
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func NewRenameCommand() *cobra.Command {
	var contextName string
	cmd := &cobra.Command{
		Use:   "rename OLD_NAME NEW_NAME",
		Short: "Rename a machine in the cluster.",
		Long: `Rename a machine in the cluster.

This command changes the name of an existing machine while preserving all other
configuration including network settings, public IP, and cluster membership.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return rename(cmd.Context(), uncli, contextName, args[0], args[1])
		},
	}
	cmd.Flags().StringVarP(
		&contextName, "context", "c", "",
		"Name of the cluster context. (default is the current context)",
	)
	return cmd
}

func rename(ctx context.Context, uncli *cli.CLI, contextName, oldName, newName string) error {
	client, err := uncli.ConnectCluster(ctx, contextName)
	if err != nil {
		return err
	}
	defer client.Close()

	machine, err := client.RenameMachine(ctx, oldName, newName)
	if err != nil {
		return fmt.Errorf("rename machine: %w", err)
	}

	// Update the machine name in the config file.
	if uncli.Config != nil {
		context := uncli.Config.Contexts[uncli.Config.CurrentContext]
		for i, c := range context.Connections {
			if c.Name == oldName {
				context.Connections[i].Name = newName
				break
			}
		}
		if err := uncli.Config.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}

	fmt.Printf("Machine %q renamed to %q (ID: %s)\n", oldName, machine.Name, machine.Id)
	return nil
}
