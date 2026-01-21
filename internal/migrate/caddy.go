package migrate

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/connector"
)

// MigrateCaddyToSystemNamespace performs a one-time migration of Caddy from the default
// namespace to the system namespace by deploying it with the correct namespace label.
// This function connects to the local API server and executes a Caddy deployment.
func MigrateCaddyToSystemNamespace(ctx context.Context, apiAddr string) error {
	addr, err := netip.ParseAddrPort(apiAddr)
	if err != nil {
		return fmt.Errorf("parse API address: %w", err)
	}

	conn := connector.NewTCPConnector(addr)
	cli, err := client.New(ctx, conn)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	defer cli.Close()

	slog.Info("Migrating Caddy to system namespace...")

	deployment, err := cli.NewCaddyDeployment("", "", api.Placement{})
	if err != nil {
		return fmt.Errorf("create Caddy deployment: %w", err)
	}

	if _, err := deployment.Run(ctx); err != nil {
		return fmt.Errorf("run Caddy deployment: %w", err)
	}

	slog.Info("Successfully migrated Caddy to system namespace")
	return nil
}
