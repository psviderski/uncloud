package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/version"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
)

func NewVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show client and server version information.",
		Long: `Show version information for both the local client and all machines in the cluster.

The client version is always shown. If connected to a cluster, the version of the
daemon running on each machine is also displayed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return runVersion(cmd.Context(), uncli)
		},
	}
	return cmd
}

type machineVersion struct {
	name    string
	state   string
	version string
}

func runVersion(ctx context.Context, uncli *cli.CLI) error {
	fmt.Printf("Client: %s\n", versionOrUnknown(version.String()))
	fmt.Println()

	// Try to connect to the cluster to get server versions.
	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		fmt.Println("Cluster: (not connected)")
		return nil
	}
	defer clusterClient.Close()

	var machines api.MachineMembersList
	var versions map[string]string

	err = spinner.New().
		Title(" Collecting version info...").
		Type(spinner.MiniDot).
		Style(lipgloss.NewStyle().Foreground(lipgloss.Color("3"))).
		ActionWithErr(func(ctx context.Context) error {
			var err error
			machines, err = clusterClient.ListMachines(ctx, nil)
			if err != nil {
				return fmt.Errorf("list machines: %w", err)
			}
			if len(machines) == 0 {
				return nil
			}
			versions, err = inspectMachineVersions(ctx, clusterClient)
			if err != nil {
				return fmt.Errorf("inspect machine versions: %w", err)
			}
			return nil
		}).
		Run()
	if err != nil {
		return err
	}

	if len(machines) == 0 {
		fmt.Println("Cluster: (no machines)")
		return nil
	}

	// Build version info for each machine.
	machineVersions := make([]machineVersion, 0, len(machines))
	for _, m := range machines {
		ver := "(unreachable)"
		if v, ok := versions[m.Machine.Name]; ok {
			ver = v
		}
		machineVersions = append(machineVersions, machineVersion{
			name:    m.Machine.Name,
			state:   capitalise(m.State.String()),
			version: ver,
		})
	}

	printVersions(machineVersions)
	return nil
}

func printVersions(machineVersions []machineVersion) {
	t := table.New().
		Border(lipgloss.Border{}).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(false).
		BorderColumn(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).PaddingRight(3)
			}
			return lipgloss.NewStyle().PaddingRight(3)
		})

	t.Headers("MACHINE", "STATE", "VERSION")

	for _, mv := range machineVersions {
		t.Row(mv.name, mv.state, mv.version)
	}

	fmt.Println(t)
}

// inspectMachineVersions broadcasts InspectMachine to all available machines and returns a map of machine name to version.
func inspectMachineVersions(ctx context.Context, c *client.Client) (map[string]string, error) {
	// Create a context that proxies to all available (non-DOWN) machines.
	proxyCtx, availableMachines, err := c.ProxyMachinesContext(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("proxy machines context: %w", err)
	}

	// Build a map of management IP to machine name for resolving response metadata.
	machineNamesByIP := make(map[string]string)
	for _, m := range availableMachines {
		if addr, err := m.Machine.Network.ManagementIp.ToAddr(); err == nil {
			machineNamesByIP[addr.String()] = m.Machine.Name
		}
	}

	// Broadcast InspectMachine to all machines.
	resp, err := c.MachineClient.InspectMachine(proxyCtx, nil)
	if err != nil {
		return nil, fmt.Errorf("inspect machines: %w", err)
	}

	versions := make(map[string]string)
	for _, details := range resp.Machines {
		var machineName string
		if details.Metadata != nil {
			machineName = machineNamesByIP[details.Metadata.Machine]
			if details.Metadata.Error != "" {
				client.PrintWarning(fmt.Sprintf("failed to get version from machine %s: %s",
					machineName, details.Metadata.Error))
				continue
			}
		} else if len(resp.Machines) == 1 && len(availableMachines) == 1 {
			// Single machine response without metadata.
			machineName = availableMachines[0].Machine.Name
		}

		if machineName != "" {
			versions[machineName] = versionOrUnknown(details.DaemonVersion)
		}
	}

	return versions, nil
}

// versionOrUnknown returns "(unknown)" if the version is empty (e.g., old daemon without version field),
// otherwise returns the version as-is.
func versionOrUnknown(v string) string {
	if v == "" {
		return "(unknown)"
	}
	return v
}

// capitalise returns a string where the first character is upper case, and the rest is lower case.
func capitalise(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}
