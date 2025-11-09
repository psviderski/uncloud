package machine

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/spf13/cobra"
)

func NewListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List machines in a cluster.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(cmd.Context(), uncli)
		},
	}
	return cmd
}

func list(ctx context.Context, uncli *cli.CLI) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	machines, err := client.ListMachines(ctx, nil)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}

	// Print the list of machines in a table format.
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	// Print header.
	if _, err = fmt.Fprintln(tw, "NAME\tSTATE\tADDRESS\tPUBLIC IP\tWIREGUARD ENDPOINTS\tMACHINE ID"); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	// Print rows.
	for _, member := range machines {
		m := member.Machine
		subnet, _ := m.Network.Subnet.ToPrefix()
		subnet = netip.PrefixFrom(network.MachineIP(subnet), subnet.Bits())

		publicIP := "-"
		if m.PublicIp != nil {
			ip, _ := m.PublicIp.ToAddr()
			publicIP = ip.String()
		}

		endpoints := make([]string, len(m.Network.Endpoints))
		for i, ep := range m.Network.Endpoints {
			addrPort, _ := ep.ToAddrPort()
			endpoints[i] = addrPort.String()
		}

		if _, err = fmt.Fprintf(
			tw, "%s\t%s\t%s\t%s\t%s\t%s\n", m.Name, capitalise(member.State.String()), subnet, publicIP,
			strings.Join(endpoints, ", "), member.Machine.Id,
		); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	return tw.Flush()
}

// capitalise returns a string where the first character is upper case, and the rest is lower case.
func capitalise(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}
