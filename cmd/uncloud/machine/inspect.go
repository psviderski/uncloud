package machine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

func NewInspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect [MACHINE]",
		Short: "Display detailed information of a machine. Without an argument it shows all machines.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			if len(args) == 0 {
				return inspectAll(cmd.Context(), uncli)
			}
			return inspect(cmd.Context(), uncli, args[0])
		},
	}

	return cmd
}

func inspect(ctx context.Context, uncli *cli.CLI, name string) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	machines, err := client.ListMachines(ctx, &api.MachineFilter{NamesOrIDs: []string{name}})
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}

	if len(machines) == 0 {
		return fmt.Errorf("machine '%s' not found", name)
	}
	data, err := json.MarshalIndent(machines[0], "", "  ")
	if err != nil {
		return fmt.Errorf("marshal machines %w", err)
	}
	fmt.Println(string(data))

	return nil
}

func inspectAll(ctx context.Context, uncli *cli.CLI) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	machines, err := client.ListMachines(ctx, &api.MachineFilter{})
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}

	// Wrap in Machines type to create array of Machines.
	type Machines struct {
		Machines api.MachineMembersList `json:"machines"`
	}
	data, err := json.MarshalIndent(Machines{machines}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal machines: %w", err)
	}
	fmt.Println(string(data))

	return nil
}
