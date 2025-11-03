package wg

import (
	"fmt"
	"strings"
	"time"

	"github.com/docker/go-units"
	"github.com/psviderski/uncloud/internal/cli"
	// "github.com/psviderski/uncloud/internal/machine/api/pb/machine"
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

			client, err := uncli.ConnectCluster(cmd.Context(), "")
			if err != nil {
				return fmt.Errorf("connection failed: %w", err)
			}
			defer client.Close()

			resp, err := client.MachineClient.GetWireGuardDevice(cmd.Context(), nil)
			if err != nil {
				return err
			}

			// Fetch the machine's name for more descriptive output
			inspectResp, err := client.Inspect(cmd.Context(), nil)
			if err != nil {
				fmt.Println("Showing WireGuard configuration:")
			} else {
				fmt.Printf("Showing WireGuard configuration for machine: %s\n", inspectResp.Name)
			}
			fmt.Println("---")

			fmt.Printf("interface: %s\n", resp.Name)
			fmt.Printf("  public key: %s\n", resp.PublicKey)
			fmt.Printf("  listen port: %d\n", resp.ListenPort)
			fmt.Println()

			for _, peer := range resp.Peers {
				fmt.Printf("peer: %s\n", peer.PublicKey)
				if peer.Endpoint != "" {
					fmt.Printf("  endpoint: %s\n", peer.Endpoint)
				}
				if peer.LastHandshakeTime != nil {
					lastHandshake := peer.LastHandshakeTime.AsTime()
					fmt.Printf("  latest handshake: %s ago\n", time.Since(lastHandshake).Round(time.Second))
				}
				fmt.Printf("  transfer: %s received, %s sent\n",
					units.HumanSize(float64(peer.ReceiveBytes)),
					units.HumanSize(float64(peer.TransmitBytes)))
				if len(peer.AllowedIps) > 0 {
					fmt.Printf("  allowed ips: %s\n", strings.Join(peer.AllowedIps, ", "))
				}
				fmt.Println()
			}

			return nil
		},
	}
	return cmd
}
