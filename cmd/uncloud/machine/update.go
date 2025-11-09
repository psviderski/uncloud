package machine

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/spf13/cobra"
)

type updateOptions struct {
	name     string
	publicIP string
}

func NewUpdateCommand() *cobra.Command {
	opts := updateOptions{}
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update machine configuration in the cluster.",
		Long: `Update machine configuration in the cluster.

This command allows setting various machine properties including:
- Machine name (--name)
- Public IP address (--public-ip)

At least one flag must be specified to perform an update operation.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return update(cmd.Context(), uncli, cmd, opts, args[0])
		},
	}

	cmd.Flags().StringVar(
		&opts.name, "name", "",
		"New name for the machine",
	)
	cmd.Flags().StringVar(
		&opts.publicIP, "public-ip", "",
		fmt.Sprintf("Public IP address of the machine for ingress configuration. Use '%s' or '' to remove the public IP.", PublicIPNone),
	)

	return cmd
}

func update(ctx context.Context, uncli *cli.CLI, cmd *cobra.Command, opts updateOptions, machineNameOrID string) error {
	// Check if at least one flag was explicitly set
	if !cmd.Flags().Changed("name") && !cmd.Flags().Changed("public-ip") {
		return fmt.Errorf("at least one update flag must be specified (--name, --public-ip)")
	}

	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	// First, resolve the machine to get its ID
	machine, err := client.InspectMachine(ctx, machineNameOrID)
	if err != nil {
		return fmt.Errorf("find machine: %w", err)
	}

	// Build the update request
	req := &pb.UpdateMachineRequest{
		MachineId: machine.Machine.Id,
	}

	if opts.name != "" {
		req.Name = &opts.name
	}

	// Check if --public-ip flag was explicitly provided
	if cmd.Flags().Changed("public-ip") {
		if opts.publicIP == "" || opts.publicIP == PublicIPNone {
			req.PublicIp = &pb.IP{} // Empty IP to signal removal
		} else {
			// Parse and validate the public IP
			ip, err := netip.ParseAddr(opts.publicIP)
			if err != nil {
				return fmt.Errorf("invalid public IP address %q: %w", opts.publicIP, err)
			}
			req.PublicIp = pb.NewIP(ip)
		}
	}

	// Perform the update operation
	updatedMachine, err := client.UpdateMachine(ctx, req)
	if err != nil {
		return fmt.Errorf("update machine: %w", err)
	}

	// Report what was changed
	changes := make([]string, 0)
	if opts.name != "" {
		changes = append(changes, fmt.Sprintf("name: %q -> %q", machine.Machine.Name, updatedMachine.Name))
	}
	if cmd.Flags().Changed("public-ip") {
		oldIP := PublicIPNone
		if machine.Machine.PublicIp != nil {
			if addr, err := machine.Machine.PublicIp.ToAddr(); err == nil {
				oldIP = addr.String()
			}
		}
		newIP := PublicIPNone
		if updatedMachine.PublicIp != nil {
			if addr, err := updatedMachine.PublicIp.ToAddr(); err == nil {
				newIP = addr.String()
			}
		}
		changes = append(changes, fmt.Sprintf("public IP: %s -> %s", oldIP, newIP))
	}

	fmt.Printf("Machine %q (ID: %s) configuration updated:\n", updatedMachine.Name, updatedMachine.Id)
	for _, change := range changes {
		fmt.Printf("  %s\n", change)
	}

	return nil
}
