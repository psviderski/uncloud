package machine

import (
	"context"
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/client"
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

type labelActionFunc func(ctx context.Context, client *client.Client, machineNameOrID string, labels []string) (*pb.MachineInfo, error)

func newLabelModifyCmd(use, short, successMsg string, action labelActionFunc) *cobra.Command {
	var contextName string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
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

			if _, err = action(cmd.Context(), client, machineNameOrID, labels); err != nil {
				return err
			}

			fmt.Printf(successMsg, machineNameOrID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&contextName, "context", "c", "", "Name of the cluster context. (default is the current context)")
	return cmd
}

func newLabelAddCmd() *cobra.Command {
	return newLabelModifyCmd(
		"add <machine> <label> [labels...]",
		"Add one or more labels to a machine",
		"Label(s) added to machine %q.\n",
		func(ctx context.Context, client *client.Client, machineNameOrID string, labels []string) (*pb.MachineInfo, error) {
			return client.AddMachineLabels(ctx, machineNameOrID, labels)
		},
	)
}

func newLabelRmCmd() *cobra.Command {
	return newLabelModifyCmd(
		"rm <machine> <label> [labels...]",
		"Remove one or more labels from a machine",
		"Label(s) removed from machine %q.\n",
		func(ctx context.Context, client *client.Client, machineNameOrID string, labels []string) (*pb.MachineInfo, error) {
			return client.RemoveMachineLabels(ctx, machineNameOrID, labels)
		},
	)
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
