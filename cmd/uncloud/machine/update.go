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
	context  string
}

func NewUpdateCommand() *cobra.Command {
	opts := updateOptions{}
	cmd := &cobra.Command{
		Use:   "update MACHINE_NAME",
		Short: "Update machine configuration in the cluster.",
		Long: `Update machine configuration in the cluster.

This command allows updating various machine properties including:
- Machine name (--name)
- Public IP address (--public-ip)

At least one flag must be specified to perform an update.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return update(cmd.Context(), uncli, opts, args[0])
		},
	}
	
	cmd.Flags().StringVar(
		&opts.name, "name", "",
		"New name for the machine",
	)
	cmd.Flags().StringVar(
		&opts.publicIP, "public-ip", "",
		"Public IP address of the machine for ingress configuration",
	)
	cmd.Flags().StringVarP(
		&opts.context, "context", "c", "",
		"Name of the cluster context. (default is the current context)",
	)
	
	return cmd
}

func update(ctx context.Context, uncli *cli.CLI, opts updateOptions, machineNameOrID string) error {
	// Validate that at least one option is provided
	if opts.name == "" && opts.publicIP == "" {
		return fmt.Errorf("at least one update flag must be specified (--name, --public-ip)")
	}
	
	client, err := uncli.ConnectCluster(ctx, opts.context)
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
	
	if opts.publicIP != "" {
		// Parse and validate the public IP
		ip, err := netip.ParseAddr(opts.publicIP)
		if err != nil {
			return fmt.Errorf("invalid public IP address %q: %w", opts.publicIP, err)
		}
		req.PublicIp = pb.NewIP(ip)
	}
	
	// Perform the update
	updatedMachine, err := client.UpdateMachine(ctx, req)
	if err != nil {
		return fmt.Errorf("update machine: %w", err)
	}
	
	// Report what was changed
	changes := []string{}
	if opts.name != "" {
		changes = append(changes, fmt.Sprintf("name: %q -> %q", machine.Machine.Name, updatedMachine.Name))
	}
	if opts.publicIP != "" {
		oldIP := "none"
		if machine.Machine.PublicIp != nil {
			if addr, err := machine.Machine.PublicIp.ToAddr(); err == nil {
				oldIP = addr.String()
			}
		}
		newIP := "none"
		if updatedMachine.PublicIp != nil {
			if addr, err := updatedMachine.PublicIp.ToAddr(); err == nil {
				newIP = addr.String()
			}
		}
		changes = append(changes, fmt.Sprintf("public IP: %s -> %s", oldIP, newIP))
	}
	
	fmt.Printf("Machine %q (ID: %s) updated:\n", updatedMachine.Name, updatedMachine.Id)
	for _, change := range changes {
		fmt.Printf("  %s\n", change)
	}
	
	return nil
}
