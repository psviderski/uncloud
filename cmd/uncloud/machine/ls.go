package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/output"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type listOptions struct {
	output string
}

func NewListCommand() *cobra.Command {
	opts := listOptions{}

	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List machines in a cluster.",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(cmd.Context(), uncli, opts)
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return output.FlagValue(opts.output)
		},
	}

	output.Flag(cmd, &opts.output)

	return cmd
}

func list(ctx context.Context, uncli *cli.CLI, opts listOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	machines, err := client.ListMachines(ctx, nil)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}

	if opts.output == "json" {
		// Wrap in Machines type to create array of Machines.
		type Machines struct {
			Machines api.MachineMembersList `json:"Machines"`
		}
		data, _ := json.MarshalIndent(Machines{machines}, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Print the list of machines in a table format.
	t := tui.NewTable()
	t.Headers("NAME", "STATE", "ADDRESS", "PUBLIC IP", "WIREGUARD ENDPOINTS", "MACHINE ID")

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

		t.Row(
			m.Name,
			capitalise(member.State.String()),
			subnet.String(),
			publicIP,
			strings.Join(endpoints, tui.Faint.Render(", ")),
			member.Machine.Id,
		)
	}

	fmt.Println(t)
	return nil
}

// capitalise returns a string where the first character is upper case, and the rest is lower case.
func capitalise(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}
