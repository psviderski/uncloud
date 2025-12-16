package machine

import (
	"fmt"
	"strings"

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
	cmd.AddCommand(
		newLabelAddCmd(),
		newLabelRmCmd(),
		newLabelLsCmd(),
	)
	return cmd
}

func newLabelAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <machine> <key=value> [key=value...]",
		Short: "Add one or more labels to a machine",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			machineNameOrID := args[0]
			labelArgs := args[1:]

			labels := make(map[string]string)
			for _, arg := range labelArgs {
				parts := strings.SplitN(arg, "=", 2)
				key := parts[0]
				val := ""
				if len(parts) > 1 {
					val = parts[1]
				}
				labels[key] = val
			}

			client, err := uncli.ConnectCluster(cmd.Context())
			if err != nil {
				return err
			}
			defer client.Close()

			if _, err = client.AddMachineLabels(cmd.Context(), machineNameOrID, labels); err != nil {
				return err
			}

			fmt.Printf("Label(s) added to machine %q.\n", machineNameOrID)
			return nil
		},
	}
}

func newLabelRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <machine> <key> [key...]",
		Short: "Remove one or more labels from a machine",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			machineNameOrID := args[0]
			labels := args[1:]

			client, err := uncli.ConnectCluster(cmd.Context())
			if err != nil {
				return err
			}
			defer client.Close()

			if _, err = client.RemoveMachineLabels(cmd.Context(), machineNameOrID, labels); err != nil {
				return err
			}

			fmt.Printf("Label(s) removed from machine %q.\n", machineNameOrID)
			return nil
		},
	}
}

func newLabelLsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ls <machine>",
		Short: "List labels on a machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			machineNameOrID := args[0]

			client, err := uncli.ConnectCluster(cmd.Context())
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
			for key, val := range labels {
				if val == "" {
					fmt.Printf("  %s\n", key)
				} else {
					fmt.Printf("  %s=%s\n", key, val)
				}
			}
			return nil
		},
	}
	return cmd
}
