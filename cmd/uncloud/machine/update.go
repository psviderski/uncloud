package machine

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/completion"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/spf13/cobra"
)

type updateOptions struct {
	name        string
	publicIP    string
	wgEndpoints []string
}

func NewUpdateCommand() *cobra.Command {
	opts := updateOptions{}
	cmd := &cobra.Command{
		Use:   "update MACHINE [flags]",
		Short: "Update machine configuration in the cluster.",
		Long: `Update machine configuration in the cluster.

Change the name, public IP address, or WireGuard endpoints of an existing machine.
At least one flag must be specified to perform an update.`,
		Example: `  # Rename a machine.
  uc machine update machine1 --name web-server

  # Set the public IP address of a machine.
  uc machine update machine1 --public-ip 203.0.113.10

  # Remove the public IP address from a machine.
  uc machine update machine1 --public-ip none

  # Update WireGuard endpoints for a machine.
  uc machine update machine1 --wg-endpoint 203.0.113.10 --wg-endpoint 192.168.1.5

  # Update multiple properties at once.
  uc machine update machine1 --name web-server --public-ip 203.0.113.10`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return update(cmd.Context(), uncli, cmd, opts, args[0])
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return completion.Machines(cmd.Context(), uncli, args, toComplete)
		},
	}

	cmd.Flags().StringVar(
		&opts.name, "name", "",
		"New name for the machine",
	)
	cmd.Flags().StringVar(
		&opts.publicIP, "public-ip", "",
		fmt.Sprintf("Public IP address of the machine for ingress configuration. Use '%s' or '' to remove the public IP.",
			PublicIPNone),
	)
	cmd.Flags().StringSliceVar(
		&opts.wgEndpoints, "wg-endpoint", nil,
		fmt.Sprintf("WireGuard endpoint address in format: IP, IP:PORT, IPv6, or [IPv6]:PORT. "+
			"Default port %d is used if omitted.\n", network.WireGuardPort)+
			"Other machines in the cluster will use this endpoint to establish a WireGuard connection to this machine.\n"+
			"Multiple endpoints can be specified by repeating the flag or using a comma-separated list.",
	)

	return cmd
}

func update(ctx context.Context, uncli *cli.CLI, cmd *cobra.Command, opts updateOptions, machineNameOrID string) error {
	// Check if at least one flag was explicitly set.
	if !cmd.Flags().Changed("name") && !cmd.Flags().Changed("public-ip") && !cmd.Flags().Changed("wg-endpoint") {
		return fmt.Errorf("at least one update flag must be specified (--name, --public-ip, --wg-endpoint)")
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

	// Parse and set endpoints if the flag was explicitly provided.
	if cmd.Flags().Changed("wg-endpoint") {
		expanded := cli.ExpandCommaSeparatedValues(opts.wgEndpoints)
		endpoints, err := cli.ParseWireGuardEndpoints(expanded)
		if err != nil {
			return err
		}
		if len(endpoints) == 0 {
			return fmt.Errorf("at least one endpoint must be specified if --wg-endpoint flag is used")
		}
		req.Endpoints = endpoints
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
	if cmd.Flags().Changed("wg-endpoint") {
		formatEndpoints := func(eps []*pb.IPPort) string {
			if len(eps) == 0 {
				return "none"
			}
			parts := make([]string, len(eps))
			for i, ep := range eps {
				ap, _ := ep.ToAddrPort()
				parts[i] = ap.String()
			}
			return strings.Join(parts, ", ")
		}
		oldEndpoints := formatEndpoints(machine.Machine.Network.Endpoints)
		newEndpoints := formatEndpoints(updatedMachine.Network.Endpoints)
		changes = append(changes, fmt.Sprintf("endpoints: %s -> %s", oldEndpoints, newEndpoints))
	}

	fmt.Printf("Machine '%s' (ID: %s) configuration updated:\n", updatedMachine.Name, updatedMachine.Id)
	for _, change := range changes {
		fmt.Printf("  %s\n", change)
	}

	return nil
}
