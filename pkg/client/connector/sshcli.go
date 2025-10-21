package connector

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/docker/cli/cli/connhelper/commandconn"
	"github.com/psviderski/uncloud/internal/machine"
	"golang.org/x/net/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// SSHCLIConnector establishes a connection to the machine API by executing SSH CLI
// and running `uncloudd dial-stdio` on the remote machine.
type SSHCLIConnector struct {
	config SSHConnectorConfig
	conn   net.Conn
}

func NewSSHCLIConnector(cfg *SSHConnectorConfig) *SSHCLIConnector {
	return &SSHCLIConnector{config: *cfg}
}

func (c *SSHCLIConnector) Connect(ctx context.Context) (*grpc.ClientConn, error) {
	// Build SSH command arguments.
	args := c.buildSSHArgs()

	// Create connection using commandconn.
	conn, err := commandconn.New(ctx, "ssh", args...)
	if err != nil {
		return nil, fmt.Errorf("SSH connection to %s@%s: %w", c.config.User, c.config.Host, err)
	}
	c.conn = conn

	// Create gRPC client over the connection.
	// Use a custom dialer that returns our existing connection.
	grpcConn, err := grpc.NewClient(
		"passthrough:///", // Dummy target since we're using a custom dialer.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return c.conn, nil
		}),
	)
	if err != nil {
		c.conn.Close()
		return nil, fmt.Errorf("create machine API client: %w", err)
	}

	return grpcConn, nil
}

// buildSSHArgs constructs the SSH command arguments.
func (c *SSHCLIConnector) buildSSHArgs() []string {
	args := []string{}

	// Add connection timeout to fail fast when node is down.
	args = append(args, "-o", "ConnectTimeout=5")

	// Add port if non-standard.
	if c.config.Port != 0 && c.config.Port != 22 {
		args = append(args, "-p", strconv.Itoa(c.config.Port))
	}

	// Add identity file if specified (backward compatibility with SSHKeyFile).
	if c.config.KeyPath != "" {
		args = append(args, "-i", c.config.KeyPath)
	}

	// Add user@host.
	args = append(args, c.config.User+"@"+c.config.Host)

	// Add remote command: uncloudd dial-stdio
	args = append(args, "uncloudd", "dial-stdio")

	// Add socket path if non-default.
	if c.config.SockPath != "" && c.config.SockPath != machine.DefaultUncloudSockPath {
		args = append(args, "--socket", c.config.SockPath)
	}

	return args
}

// Dialer returns a proxy dialer for establishing connections within the cluster through the SSH tunnel.
func (c *SSHCLIConnector) Dialer() (proxy.ContextDialer, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("SSH connection must be established first")
	}
	// For SSH CLI connector, we can't provide a generic dialer since we only have
	// a single connection to the dial-stdio process.
	// This matches the limitation of the old SSH connector.
	return nil, fmt.Errorf("proxy connections are not supported over SSH CLI connector")
}

func (c *SSHCLIConnector) Close() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}
