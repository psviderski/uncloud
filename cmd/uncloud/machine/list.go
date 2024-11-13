package machine

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
	"net/netip"
	"os"
	"strings"
	"text/tabwriter"
	"uncloud/internal/cli"
	"uncloud/internal/machine/network"
	"uncloud/internal/secret"
)

func NewListCommand() *cobra.Command {
	var cluster string
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List machines in a cluster.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return runList(cmd.Context(), uncli, cluster)
		},
	}
	cmd.Flags().StringVarP(
		&cluster, "cluster", "c", "",
		"Name of the cluster. (default is the current cluster)",
	)
	return cmd
}

func runList(ctx context.Context, uncli *cli.CLI, clusterName string) error {
	c, err := uncli.ConnectCluster(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer func() {
		_ = c.Close()
	}()

	listResp, err := c.ListMachines(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}

	// Print the list of machines in a table format.
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	// Print header.
	if _, err = fmt.Fprintln(tw, "NAME\tSTATE\tADDRESS\tPUBLIC KEY\tENDPOINTS"); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	// Print rows.
	for _, member := range listResp.Machines {
		m := member.Machine
		subnet, _ := m.Network.Subnet.ToPrefix()
		subnet = netip.PrefixFrom(network.MachineIP(subnet), subnet.Bits())
		endpoints := make([]string, len(m.Network.Endpoints))
		for i, ep := range m.Network.Endpoints {
			addrPort, _ := ep.ToAddrPort()
			endpoints[i] = addrPort.String()
		}
		publicKey := secret.Secret(m.Network.PublicKey)
		if _, err = fmt.Fprintf(
			tw, "%s\t%s\t%s\t%s\t%s\n", m.Name, capitalise(member.State.String()), subnet, publicKey, strings.Join(endpoints, ", "),
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
