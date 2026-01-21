package wg

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/docker/go-units"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wg",
		Short: "Inspect WireGuard network",
	}
	cmd.AddCommand(newShowCommand())
	return cmd
}

type showOptions struct {
	machine string
}

func newShowCommand() *cobra.Command {
	opts := showOptions{}
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show WireGuard network configuration for a machine.",
		Long: "Show the WireGuard network configuration for the machine currently connected to " +
			"(or specified by the global --connect flag).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return runShow(cmd.Context(), uncli, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.machine, "machine", "m", "",
		"Name or ID of the machine to show the configuration for. (default is connected machine)")
	return cmd
}

func runShow(ctx context.Context, uncli *cli.CLI, opts showOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer client.Close()

	if opts.machine != "" {
		// Proxy requests to the specified machine.
		ctx, _, err = client.ProxyMachinesContext(ctx, []string{opts.machine})
		if err != nil {
			return err
		}
	}

	resp, err := client.MachineClient.InspectWireGuardNetwork(ctx, nil)
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return fmt.Errorf("inspect WireGuard network: "+
				"make sure the target machine is running uncloudd daemon version >= 0.16.0: %w", err)
		}
		return err
	}

	machines, err := client.ListMachines(ctx, nil)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}
	machinesNamesByPublicKey := make(map[string]string)
	for _, m := range machines {
		publicKey := wgtypes.Key(m.Machine.Network.PublicKey).String()
		machinesNamesByPublicKey[publicKey] = m.Machine.Name
	}

	// Fetch the machine's name for more descriptive output
	inspectResp, err := client.MachineClient.InspectMachine(ctx, nil)
	if err == nil {
		fmt.Printf("Machine name:         %s\n", inspectResp.Machines[0].Machine.Name)
	}

	fmt.Printf("WireGuard interface:  %s\n", resp.InterfaceName)
	fmt.Printf("WireGuard public key: %s\n", wgtypes.Key(resp.PublicKey).String())
	fmt.Printf("WireGuard port:       %d\n", resp.ListenPort)
	fmt.Println()

	if len(resp.Peers) == 0 {
		fmt.Println("No WireGuard peers configured.")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	if _, err = fmt.Fprintln(tw, "PEER\tPUBLIC KEY\tENDPOINT\tHANDSHAKE\tRECEIVED\tSENT\tALLOWED IPS"); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	for _, peer := range resp.Peers {
		machineName, ok := machinesNamesByPublicKey[wgtypes.Key(peer.PublicKey).String()]
		if !ok {
			machineName = "(unknown)"
		}

		lastHandshake := ""
		if peer.LastHandshakeTime != nil {
			lastHandshake = time.Since(peer.LastHandshakeTime.AsTime()).Round(time.Second).String() + " ago"
		}

		_, err = fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			machineName,
			wgtypes.Key(peer.PublicKey).String(),
			peer.Endpoint,
			lastHandshake,
			units.HumanSize(float64(peer.ReceiveBytes)),
			units.HumanSize(float64(peer.TransmitBytes)),
			strings.Join(peer.AllowedIps, ", "),
		)
		if err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	return tw.Flush()
}
