package migrate

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/connector"
)

// MigrateCaddyToSystemNamespace performs a one-time migration of Caddy from the default
// namespace to the system namespace by deploying it with the correct namespace label.
// This function connects to the local API server and executes a Caddy deployment.
func MigrateCaddyToSystemNamespace(ctx context.Context, apiAddr string) error {
	// Parse the API address
	addr, err := netip.ParseAddrPort(apiAddr)
	if err != nil {
		return fmt.Errorf("parse API address: %w", err)
	}

	// Give the server a moment to fully start accepting connections
	time.Sleep(100 * time.Millisecond)

	// Create a TCP connector to the local API server
	conn := connector.NewTCPConnector(addr)
	cli, err := client.New(ctx, conn)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	defer cli.Close()

	slog.Info("Migrating Caddy to system namespace...")

	// Create a Caddy deployment with empty image (will use latest) and placement
	deployment, err := cli.NewCaddyDeployment("", "", api.Placement{})
	if err != nil {
		return fmt.Errorf("create Caddy deployment: %w", err)
	}

	// Run the deployment - this will create new containers in system namespace
	// and remove old containers from default namespace via rolling update
	if _, err := deployment.Run(ctx); err != nil {
		return fmt.Errorf("run Caddy deployment: %w", err)
	}

	slog.Info("Successfully migrated Caddy to system namespace")
	return nil
}
