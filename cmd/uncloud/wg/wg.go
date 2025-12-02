package wg

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/docker/go-units"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wg",
		Short: "Inspect WireGuard network",
	}
	cmd.AddCommand(newShowCommand())
	return cmd
}

func newShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show WireGuard configuration for the current machine",
		Long:  "Shows the WireGuard configuration for the machine currently connected to (or specified by the global --connect flag).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			ctx := cmd.Context()

			client, err := uncli.ConnectCluster(ctx)
			if err != nil {
				return fmt.Errorf("connection failed: %w", err)
			}
			defer client.Close()

			resp, err := client.MachineClient.GetWireGuardDevice(ctx, nil)
			if err != nil {
				return err
			}

			machines, err := client.ListMachines(ctx, nil)
			if err != nil {
				return fmt.Errorf("list machines: %w", err)
			}
			machinesNamesByPublicKey := make(map[string]string)
			for _, m := range machines {
				publicKey := base64.StdEncoding.EncodeToString(m.Machine.Network.PublicKey)
				machinesNamesByPublicKey[publicKey] = m.Machine.Name
			}

			// Fetch the machine's name for more descriptive output
			inspectResp, err := client.Inspect(cmd.Context(), nil)
			if err == nil {
				fmt.Printf("Machine Name:         %s\n", inspectResp.Name)
			}

			fmt.Printf("WireGuard interface:  %s\n", resp.Name)
			fmt.Printf("WireGuard public key: %s\n", resp.PublicKey)
			fmt.Printf("WireGuard port:       %d\n", resp.ListenPort)
			fmt.Println()

			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			if _, err = fmt.Fprintln(tw, "MACHINE\tPUBLIC KEY\tENDPOINT\tHANDSHAKE\tRECEIVED\tSENT\tALLOWED IPS"); err != nil {
				return fmt.Errorf("write header: %w", err)
			}

			for _, peer := range resp.Peers {
				machineName, ok := machinesNamesByPublicKey[peer.PublicKey]
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
					peer.PublicKey,
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
		},
	}
	return cmd
}
