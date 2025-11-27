package machine

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/output"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/spf13/cobra"
)

type listOptions struct {
	format string
}

func NewListCommand() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List machines in a cluster.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(cmd.Context(), uncli, opts)
		},
	}
	cmd.Flags().StringVar(&opts.format, "format", "table", "Output format (table, json)")
	return cmd
}

type machineItem struct {
	Name               string   `json:"name"`
	State              string   `json:"state"`
	Address            string   `json:"address"`
	PublicIP           *string  `json:"publicIp"`
	WireGuardEndpoints []string `json:"wireguardEndpoints"`
	MachineID          string   `json:"machineId"`
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

	var items []machineItem
	for _, member := range machines {
		m := member.Machine
		subnet, _ := m.Network.Subnet.ToPrefix()
		subnet = netip.PrefixFrom(network.MachineIP(subnet), subnet.Bits())

		var publicIP *string
		if m.PublicIp != nil {
			ip, _ := m.PublicIp.ToAddr()
			s := ip.String()
			publicIP = &s
		}

		endpoints := make([]string, len(m.Network.Endpoints))
		for i, ep := range m.Network.Endpoints {
			addrPort, _ := ep.ToAddrPort()
			endpoints[i] = addrPort.String()
		}

		items = append(items, machineItem{
			Name:               m.Name,
			State:              member.State.String(),
			Address:            subnet.String(),
			PublicIP:           publicIP,
			WireGuardEndpoints: endpoints,
			MachineID:          m.Id,
		})
	}

	columns := []output.Column[machineItem]{
		{Header: "NAME", Field: "Name"},
		{
			Header: "STATE",
			Accessor: func(r machineItem) string {
				return capitalise(r.State)
			},
		},
		{Header: "ADDRESS", Field: "Address"},
		{Header: "PUBLIC IP", Field: "PublicIP"},
		{Header: "WIREGUARD ENDPOINTS", Field: "WireGuardEndpoints"},
		{Header: "MACHINE ID", Field: "MachineID"},
	}

	return output.Print(items, columns, opts.format)
}

// capitalise returns a string where the first character is upper case, and the rest is lower case.
func capitalise(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}
