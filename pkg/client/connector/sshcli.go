package connector

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
	// Path to SSH control socket for connection reuse.
	controlSockPath string
}

func NewSSHCLIConnector(cfg *SSHConnectorConfig) *SSHCLIConnector {
	return &SSHCLIConnector{
		config:          *cfg,
		controlSockPath: controlSocketPath(),
	}
}

// controlSocketPath returns a unique control socket path for the SSH connection.
// Returns an empty string if unable to find or create a suitable path.
func controlSocketPath() string {
	// %C is expanded by `ssh` to a hash of user, local and remote hostnames, port, and the contents
	// of the ProxyJump option. This ensures that shared connections are uniquely identified.
	sockName := fmt.Sprintf("uc_control_%%C.sock")

	// Prefer XDG_RUNTIME_DIR if set, fall back to ~/.ssh if it exists.
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, sockName)
	}
	if home, err := os.UserHomeDir(); err == nil {
		sshDir := filepath.Join(home, ".ssh")
		if fi, sErr := os.Stat(sshDir); sErr == nil && fi.IsDir() {
			return filepath.Join(sshDir, sockName)
		}
	}

	// Last resort: create a subdirectory in temp with restricted permissions.
	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("uncloud-%d", os.Getuid()))
	path := filepath.Join(tmpDir, sockName)
	if len(path)-2+40 < 104 { // 40 chars for %C hash, 104 is typical UNIX socket path limit
		if err := os.MkdirAll(tmpDir, 0o700); err == nil {
			return path
		}
	}

	return ""
}

func (c *SSHCLIConnector) Connect(ctx context.Context) (*grpc.ClientConn, error) {
	// Create gRPC client with a dialer that spawns a new SSH connection on demand.
	// Each dial attempt runs `ssh ... uncloudd dial-stdio`, reusing the control socket if available.
	grpcConn, err := grpc.NewClient(
		"passthrough:///", // Dummy target since we're using a custom dialer.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(defaultServiceConfig),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			args := c.buildSSHArgs()
			conn, err := commandconn.New(ctx, "ssh", args...)
			if err != nil {
				return nil, fmt.Errorf("SSH connection to %s: %w", c.config.Destination(), err)
			}
			return conn, nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create machine API client: %w", err)
	}

	return grpcConn, nil
}

// buildSSHArgs constructs the SSH command arguments to run `uncloudd dial-stdio` on the remote machine reusing
// the established connection via control socket.
func (c *SSHCLIConnector) buildSSHArgs() []string {
	var args []string

	// Add control socket options for connection reuse if available.
	if c.controlSockPath != "" {
		args = append(args, "-o", "ControlMaster=auto")
		args = append(args, "-o", "ControlPath="+c.controlSockPath)

		// Keep the established connection alive for a short duration after the last session closes to allow reuse.
		controlPersist := "10m"
		// Override the default duration with the UNCLOUD_SSH_CONTROL_PERSIST env variable.
		if v := os.Getenv("UNCLOUD_SSH_CONTROL_PERSIST"); v != "" {
			controlPersist = v
		}
		args = append(args, "-o", "ControlPersist="+controlPersist)
	}

	// Add connection timeout to fail fast when node is down.
	args = append(args, "-o", "ConnectTimeout=5")
	// Disable pseudo-terminal allocation to prevent SSH from executing as a login shell.
	args = append(args, "-T")

	// Add port if specified.
	if c.config.Port != 0 {
		args = append(args, "-p", strconv.Itoa(c.config.Port))
	}

	// Add identity file if specified (backward compatibility with SSHKeyFile).
	if c.config.KeyPath != "" {
		args = append(args, "-i", c.config.KeyPath)
	}

	// Add [user@]host destination.
	args = append(args, c.config.Destination())

	// Add remote command: uncloudd dial-stdio
	args = append(args, "uncloudd", "dial-stdio")

	// Add socket path if non-default.
	if c.config.SockPath != "" && c.config.SockPath != machine.DefaultUncloudSockPath {
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
		config:          c.config,
		controlSockPath: c.controlSockPath,
	}, nil
}

func (c *SSHCLIConnector) Close() error {
	// Individual connections are managed by gRPC and closed when the gRPC connection closes.
	// The SSH control socket may persist for connection reuse across CLI invocations.
	return nil
}

// sshCLIDialer implements proxy.ContextDialer by spawning SSH processes with -W flag.
type sshCLIDialer struct {
	config SSHConnectorConfig
	// Shared control socket path from SSHCLIConnector for connection reuse.
	controlSockPath string
}

// buildDialArgs constructs SSH command arguments for -W flag dialing.
func (d *sshCLIDialer) buildDialArgs(address string) []string {
	var args []string

	if d.controlSockPath != "" {
		// Try to reuse the existing control connection without initiating a new one.
		// Falls back to direct connection if the control socket is not available.
		args = append(args, "-o", "ControlMaster=no")
		args = append(args, "-o", "ControlPath="+d.controlSockPath)
	}

	// Add connection timeout to fail fast when node is down.
	args = append(args, "-o", "ConnectTimeout=5")
	// Disable pseudo-terminal allocation to prevent SSH from executing as a login shell.
	args = append(args, "-T")

	// Add port if specified.
	if d.config.Port != 0 {
		args = append(args, "-p", strconv.Itoa(d.config.Port))
	}

	// Add identity file if specified.
	if d.config.KeyPath != "" {
		args = append(args, "-i", d.config.KeyPath)
	}

	// Add -W flag for stdin/stdout forwarding to target address.
	args = append(args, "-W", address)

	// Add [user@]host destination.
	args = append(args, d.config.Destination())

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

	// Create connection using docker's commandconn.
	conn, err := commandconn.New(ctx, "ssh", args...)
	if err != nil {
		return nil, fmt.Errorf("SSH connection to %s for dialing %s: %w", d.config.Destination(), address, err)
	}

	return conn, nil
}
