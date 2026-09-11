package machine

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/spf13/cobra"
)

type updateOptions struct {
	name        string
	publicIP    string
	wgEndpoints []string
	wgPort      int
	yes         bool
}

func NewUpdateCommand() *cobra.Command {
	opts := updateOptions{}
	cmd := &cobra.Command{
		Use:   "update MACHINE [flags]",
		Short: "Update machine configuration in the cluster.",
		Long: `Update machine configuration in the cluster.

Change the name, public IP address, WireGuard endpoints, or WireGuard listen port of an existing machine.
At least one flag must be specified to perform an update.`,
		Example: `  # Rename a machine.
  uc machine update machine1 --name web-server

  # Set the public IP address of a machine.
  uc machine update machine1 --public-ip 203.0.113.10

  # Remove the public IP address from a machine.
  uc machine update machine1 --public-ip none

  # Update WireGuard endpoints for a machine.
  uc machine update machine1 --wg-endpoint 203.0.113.10 --wg-endpoint 192.168.1.5

  # Change the WireGuard listen port. Advertised endpoints using the old port are adjusted automatically.
  uc machine update machine1 --wg-port 51821

  # Change the listen port and set explicit endpoints (e.g. behind NAT with port forwarding).
  uc machine update machine1 --wg-port 51821 --wg-endpoint 203.0.113.10:9821

  # Update multiple properties at once.
  uc machine update machine1 --name web-server --public-ip 203.0.113.10`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			cli.BindEnvToFlag(cmd, "yes", "UNCLOUD_AUTO_CONFIRM")
			return nil
		},
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
		"WireGuard endpoint address that other machines in the cluster should use to establish "+
			"WireGuard connections\n"+
			"to this machine. This doesn't change the port WireGuard listens on the machine (see --wg-port).\n"+
			"Format: IP, IP:PORT, IPv6, or [IPv6]:PORT. Default port is the value of --wg-port (or the current\n"+
			"listen port) if omitted.\n"+
			"Multiple endpoints can be specified by repeating the flag or using a comma-separated list.",
	)
	cmd.Flags().IntVar(
		&opts.wgPort, "wg-port", network.DefaultWireGuardPort,
		"UDP port WireGuard listens on for incoming connections from other machines.\n"+
			"Advertised endpoints using the old listen port are adjusted to the new port automatically\n"+
			"unless --wg-endpoint is provided.",
	)
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false,
		"Auto-confirm the WireGuard listen port change prompt.\n"+
			"Should be explicitly set when running non-interactively, e.g., in CI/CD pipelines. [$UNCLOUD_AUTO_CONFIRM]")

	return cmd
}

func update(ctx context.Context, uncli *cli.CLI, cmd *cobra.Command, opts updateOptions, machineNameOrID string) error {
	// Check if at least one flag was explicitly set.
	if !cmd.Flags().Changed("name") && !cmd.Flags().Changed("public-ip") &&
		!cmd.Flags().Changed("wg-endpoint") && !cmd.Flags().Changed("wg-port") {
		return fmt.Errorf("at least one update flag must be specified (--name, --public-ip, --wg-endpoint, --wg-port)")
	}

	if cmd.Flags().Changed("wg-port") && (opts.wgPort < 1 || opts.wgPort > 65535) {
		return fmt.Errorf("invalid WireGuard port %d: must be between 1 and 65535", opts.wgPort)
	}

	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	// Resolve the machine to capture its current configuration for the before/after report and to validate existence.
	machine, err := client.InspectMachine(ctx, machineNameOrID)
	if err != nil {
		return fmt.Errorf("find machine: %w", err)
	}

	req := &pb.UpdateMachineRequest{}

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

	// The listen port doubles as the default port for bare-IP endpoints so that a combined
	// --wg-port + --wg-endpoint change stays consistent. Fall back to the default WireGuard port
	// when --wg-port is not provided.
	endpointDefaultPort := uint16(network.DefaultWireGuardPort)
	if cmd.Flags().Changed("wg-port") {
		port := int32(opts.wgPort)
		req.WireguardPort = &port
		endpointDefaultPort = uint16(opts.wgPort)
	}

	// Parse and set endpoints if the flag was explicitly provided.
	if cmd.Flags().Changed("wg-endpoint") {
		expanded := cli.ExpandCommaSeparatedValues(opts.wgEndpoints)
		endpoints, err := cli.ParseWireGuardEndpoints(expanded, endpointDefaultPort)
		if err != nil {
			return err
		}
		if len(endpoints) == 0 {
			return fmt.Errorf("at least one endpoint must be specified if --wg-endpoint flag is used")
		}
		req.Endpoints = endpoints
	}

	// Changing the WireGuard listen port reconfigures the network the machine is managed over.
	// Confirm the change (unless auto-confirmed) and warn if the machine risks becoming unreachable.
	if cmd.Flags().Changed("wg-port") && !opts.yes {
		if !cmd.Flags().Changed("wg-endpoint") {
			fmt.Printf("Changing the WireGuard listen port of machine '%s' to %d.\n"+
				"Advertised endpoints using the old port will be adjusted to the new port automatically.\n",
				machine.Machine.Name, opts.wgPort)
		} else {
			fmt.Printf("Changing the WireGuard listen port of machine '%s' to %d "+
				"and setting explicit endpoints.\n", machine.Machine.Name, opts.wgPort)
		}

		confirmed, err := tui.Confirm("Proceed with the WireGuard listen port change?")
		if err != nil {
			return fmt.Errorf("confirm WireGuard port change: %w", err)
		}
		if !confirmed {
			return cli.Cancelled("Machine update cancelled.")
		}
	}

	updatedMachine, err := client.UpdateMachine(ctx, machine.Machine.Id, req)
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
	if cmd.Flags().Changed("wg-port") {
		changes = append(changes, fmt.Sprintf("WireGuard port: -> %d", opts.wgPort))
	}
	if cmd.Flags().Changed("wg-endpoint") || cmd.Flags().Changed("wg-port") {
		oldEndpoints := formatEndpoints(machine.Machine.Network.Endpoints)
		newEndpoints := formatEndpoints(updatedMachine.Network.Endpoints)
		if oldEndpoints != newEndpoints {
			changes = append(changes, fmt.Sprintf("endpoints: %s -> %s", oldEndpoints, newEndpoints))
		}
	}

	fmt.Printf("Machine '%s' (ID: %s) configuration updated:\n", updatedMachine.Name, updatedMachine.Id)
	for _, change := range changes {
		fmt.Printf("  %s\n", change)
	}

	// Warn if a listen port change left no advertised endpoint using the new port. Other machines would
	// be unable to establish new WireGuard connections to this machine on the new port.
	if cmd.Flags().Changed("wg-port") {
		if !endpointsContainPort(updatedMachine.Network.Endpoints, uint16(opts.wgPort)) {
			fmt.Printf("\nWarning: no advertised WireGuard endpoint of machine '%s' uses the new listen port %d.\n"+
				"Other machines may be unable to connect to it. Set matching endpoints with:\n"+
				"  uc machine update %s --wg-endpoint <IP>:%d\n",
				updatedMachine.Name, opts.wgPort, updatedMachine.Name, opts.wgPort)
		}
	}

	return nil
}

// formatEndpoints renders a list of IPPort endpoints as a comma-separated string, or "none" if empty.
func formatEndpoints(eps []*pb.IPPort) string {
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

// endpointsContainPort reports whether any endpoint in the list uses the given port.
func endpointsContainPort(eps []*pb.IPPort, port uint16) bool {
	for _, ep := range eps {
		ap, err := ep.ToAddrPort()
		if err != nil {
			continue
		}
		if ap.Port() == port {
			return true
		}
	}
	return false
}
