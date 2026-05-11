package wg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/go-units"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
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

	completion.MachinesFlag(cmd)

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
	machinesByPublicKey := make(map[string]*pb.MachineInfo)
	for _, m := range machines {
		publicKey := wgtypes.Key(m.Machine.Network.PublicKey).String()
		machinesByPublicKey[publicKey] = m.Machine
	}

	// Fetch the machine's info and RTTs for display.
	var selfMachine *pb.MachineDetails
	inspectResp, err := client.MachineClient.InspectMachine(ctx, nil)
	if err == nil {
		selfMachine = inspectResp.Machines[0]
		fmt.Printf("Machine name:         %s\n", selfMachine.Machine.Name)
	}

	fmt.Printf("WireGuard interface:  %s\n", resp.InterfaceName)
	fmt.Printf("WireGuard public key: %s\n", wgtypes.Key(resp.PublicKey).String())
	fmt.Printf("WireGuard port:       %d\n", resp.ListenPort)
	fmt.Println()

	if len(resp.Peers) == 0 {
		fmt.Println("No WireGuard peers configured.")
		return nil
	}

	t := tui.NewTable()
	t.Headers("PEER", "PUBLIC KEY", "ENDPOINT", "HANDSHAKE", "RTT", "RECEIVED", "SENT", "ALLOWED IPS")

	for _, peer := range resp.Peers {
		publicKeyStr := wgtypes.Key(peer.PublicKey).String()
		machineName := "(unknown)"
		rtt := "-"
		if m, ok := machinesByPublicKey[publicKeyStr]; ok {
			machineName = m.Name
			if selfMachine != nil {
				if stats, ok := selfMachine.Rtts[m.Id]; ok {
					rtt = tui.FormatRTT(stats.Median.AsDuration())
				}
			}
		}

		lastHandshake := ""
		if peer.LastHandshakeTime != nil {
			lastHandshake = time.Since(peer.LastHandshakeTime.AsTime()).Round(time.Second).String() + " ago"
		}

		t.Row(
			machineName,
			publicKeyStr,
			peer.Endpoint,
			lastHandshake,
			rtt,
			units.HumanSize(float64(peer.ReceiveBytes)),
			units.HumanSize(float64(peer.TransmitBytes)),
			strings.Join(peer.AllowedIps, tui.Faint.Render(", ")),
		)
	}

	fmt.Println(t)
	return nil
}
