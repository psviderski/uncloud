package main

import (
	"context"
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/version"
	"github.com/spf13/cobra"
)

func NewVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show client and server version information.",
		Long: `Show version information for both the local client and the connected server.

The client version is always shown. If connected to a cluster, the server (daemon)
version is also displayed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return runVersion(cmd.Context(), uncli)
		},
	}
	return cmd
}

func runVersion(ctx context.Context, uncli *cli.CLI) error {
	fmt.Printf("Client: %s\n", versionOrDev(version.String()))

	// Try to connect to the cluster to get the server version.
	clusterClient, err := uncli.ConnectClusterWithOptions(ctx, cli.ConnectOptions{
		ShowProgress: false,
	})
	if err != nil {
		fmt.Printf("Server: %s\n", "(not connected)")
		return nil
	}
	defer clusterClient.Close()

	serverVersion, err := clusterClient.GetVersion(ctx)
	if err != nil {
		fmt.Printf("Server: %s\n", "(unavailable)")
		return nil
	}

	fmt.Printf("Server: %s\n", versionOrDev(serverVersion))
	return nil
}

// versionOrDev returns "(dev)" if the version is empty, otherwise returns the version as-is.
func versionOrDev(v string) string {
	if v == "" {
		return "(dev)"
	}
	return v
}
