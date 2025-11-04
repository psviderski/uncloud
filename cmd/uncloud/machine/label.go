package machine

import (
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func newLabelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Manage machine labels",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("a valid subcommand is required")
		},
	}
	cmd.AddCommand(newLabelAddCmd())
	cmd.AddCommand(newLabelRmCmd())
	cmd.AddCommand(newLabelLsCmd())
	return cmd
}

func newLabelAddCmd() *cobra.Command {
	var contextName string
	cmd := &cobra.Command{
		Use:   "add <machine> <label> [labels...]",
		Short: "Add one or more labels to a machine",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			machineNameOrID := args[0]
			labels := args[1:]

			client, err := uncli.ConnectCluster(cmd.Context(), contextName)
			if err != nil {
				return err
			}
			defer client.Close()

			if _, err = client.AddMachineLabels(cmd.Context(), machineNameOrID, labels); err != nil {
				return fmt.Errorf("add labels to machine: %w", err)
			}

			fmt.Printf("Label(s) added to machine %q.\n", machineNameOrID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&contextName, "context", "c", "", "Name of the cluster context. (default is the current context)")
	return cmd
}

func newLabelRmCmd() *cobra.Command {
	var contextName string
	cmd := &cobra.Command{
		Use:   "rm <machine> <label> [labels...]",
		Short: "Remove one or more labels from a machine",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			machineNameOrID := args[0]
			labels := args[1:]

			client, err := uncli.ConnectCluster(cmd.Context(), contextName)
			if err != nil {
				return err
			}
			defer client.Close()

			if _, err = client.RemoveMachineLabels(cmd.Context(), machineNameOrID, labels); err != nil {
				return fmt.Errorf("remove labels from machine: %w", err)
			}

			fmt.Printf("Label(s) removed from machine %q.\n", machineNameOrID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&contextName, "context", "c", "", "Name of the cluster context. (default is the current context)")
	return cmd
}

func newLabelLsCmd() *cobra.Command {
	var contextName string
	cmd := &cobra.Command{
		Use:   "ls <machine>",
		Short: "List labels on a machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			machineNameOrID := args[0]

			client, err := uncli.ConnectCluster(cmd.Context(), contextName)
			if err != nil {
				return err
			}
			defer client.Close()

			labels, err := client.GetMachineLabels(cmd.Context(), machineNameOrID)
			if err != nil {
				return fmt.Errorf("list labels for machine: %w", err)
			}

			if len(labels) == 0 {
				fmt.Printf("No labels found for machine %q.\n", machineNameOrID)
				return nil
			}

			fmt.Printf("Labels for machine %q:\n", machineNameOrID)
			for _, label := range labels {
				fmt.Printf("  %s\n", label)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&contextName, "context", "c", "", "Name of the cluster context. (default is the current context)")
	return cmd
}
