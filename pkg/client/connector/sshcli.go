package connector

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/docker/cli/cli/connhelper/commandconn"
	"github.com/psviderski/uncloud/internal/machine/constants"
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

// sshCLIDialer implements proxy.ContextDialer by spawning SSH processes with -W flag.
type sshCLIDialer struct {
	config SSHConnectorConfig
}

// buildDialArgs constructs SSH command arguments for -W flag dialing.
func (d *sshCLIDialer) buildDialArgs(address string) []string {
	args := []string{}

	// Add connection timeout to fail fast when node is down.
	args = append(args, "-o", "ConnectTimeout=5")

	// Add port if non-standard.
	if d.config.Port != 0 && d.config.Port != 22 {
		args = append(args, "-p", strconv.Itoa(d.config.Port))
	}

	// Add identity file if specified.
	if d.config.KeyPath != "" {
		args = append(args, "-i", d.config.KeyPath)
	}

	// Add -W flag for stdin/stdout forwarding to target address.
	args = append(args, "-W", address)

	// Add user@host.
	args = append(args, d.config.User+"@"+d.config.Host)

	return args
}

// DialContext establishes a connection to the target address through an SSH tunnel using -W flag.
func (d *sshCLIDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// Only support TCP connections.
	if network != "tcp" {
		return nil, fmt.Errorf("unsupported network type: %s", network)
	}

	// Build SSH command arguments.
	args := d.buildDialArgs(address)

	// Create connection using commandconn.
	conn, err := commandconn.New(ctx, "ssh", args...)
	if err != nil {
		return nil, fmt.Errorf("SSH connection to %s@%s for dialing %s: %w", d.config.User, d.config.Host, address, err)
	}

	return conn, nil
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
		grpc.WithDefaultServiceConfig(defaultServiceConfig),
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
	if c.config.SockPath != "" && c.config.SockPath != constants.DefaultUncloudSockPath {
		args = append(args, "--socket", c.config.SockPath)
	}

	return args
}

// Dialer returns a proxy dialer for establishing connections within the cluster through SSH tunnels.
func (c *SSHCLIConnector) Dialer() (proxy.ContextDialer, error) {
	if c.config == (SSHConnectorConfig{}) {
		return nil, fmt.Errorf("SSH connector not configured")
	}

	return &sshCLIDialer{
		config: c.config,
	}, nil
}

func (c *SSHCLIConnector) Close() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}
