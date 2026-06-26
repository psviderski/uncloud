package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/spf13/cobra"
)

func NewListCommand() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List machines in a cluster.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(cmd.Context(), uncli, output)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "",
		"Output format: 'json' or empty for a human-readable table.")

	return cmd
}

func list(ctx context.Context, uncli *cli.CLI, output string) error {
	if output != "" && output != "json" {
		return fmt.Errorf("unsupported output format '%s' (supported: json)", output)
	}

	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	machines, err := client.ListMachines(ctx, nil)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}

	if output == "json" {
		data, err := json.MarshalIndent(machines.ToNative(), "", "  ")
		if err != nil {
			return fmt.Errorf("marshal machines: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Print the list of machines in a table format.
	t := tui.NewTable()
	t.Headers("NAME", "STATE", "ADDRESS", "PUBLIC IP", "WIREGUARD ENDPOINTS",
		"OS", "KERNEL", "ARCH", "DOCKER", "VERSION")

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

		arch := "-"
		if m.Arch != "" {
			arch = m.Arch
		}

		osName := "-"
		if m.OsPrettyName != "" {
			osName = m.OsPrettyName
		}

		kernel := "-"
		if m.KernelVersion != "" {
			kernel = m.KernelVersion
		}

		daemonVersion := "-"
		if m.DaemonVersion != "" {
			daemonVersion = m.DaemonVersion
		}

		dockerVersion := "-"
		if m.DockerVersion != "" {
			dockerVersion = m.DockerVersion
		}

		t.Row(
			m.Name,
			capitalise(member.State.String()),
			subnet.String(),
			publicIP,
			strings.Join(endpoints, tui.Faint.Render(", ")),
			osName,
			kernel,
			arch,
			dockerVersion,
			daemonVersion,
		)
	}

	lipgloss.Println(t)
	return nil
}

// capitalise returns a string where the first character is upper case, and the rest is lower case.
func capitalise(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}
