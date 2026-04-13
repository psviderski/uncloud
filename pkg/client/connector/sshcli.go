package connector

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/cli/cli/connhelper/commandconn"
	"github.com/psviderski/uncloud/internal/grpcversion"
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
	// fwdCheckOnce ensures the TCP forwarding check runs only once.
	fwdCheckOnce sync.Once
	// fwdCheckErr caches the result of the TCP forwarding check.
	fwdCheckErr error
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
	// Validate SSH connectivity by running a no-op command on the remote machine. This also
	// establishes the control socket (ControlMaster=auto) so subsequent connections reuse it.
	probeArgs := append(c.buildSSHArgs(), "true")
	probe := exec.CommandContext(ctx, "ssh", probeArgs...)
	if output, err := probe.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("SSH connection to '%s': %w: %s",
			c.config.Destination(), err, strings.TrimSpace(string(output)))
	}

	// Create gRPC client with a dialer that spawns new SSH connections on demand,
	// reusing the control socket established above.
	grpcConn, err := grpc.NewClient(
		"passthrough:///", // Dummy target since we're using a custom dialer.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(defaultServiceConfig),
		grpc.WithUnaryInterceptor(grpcversion.ClientUnaryInterceptor),
		grpc.WithStreamInterceptor(grpcversion.ClientStreamInterceptor),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			dialArgs := append(c.buildSSHArgs(), "uncloudd", "dial-stdio")
			if c.config.SockPath != "" {
				dialArgs = append(dialArgs, "--socket", c.config.SockPath)
			}

			conn, err := commandconn.New(ctx, "ssh", dialArgs...)
			if err != nil {
				return nil, fmt.Errorf("SSH connection to '%s': %w", c.config.Destination(), err)
			}
			return conn, nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create machine API client: %w", err)
	}

	return grpcConn, nil
}

// buildSSHArgs constructs the SSH command arguments with connection options and destination. The options
// include control socket settings for connection reuse if necessary.
// The remote command is not included and should be appended by the caller.
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
	// Disable interactive prompts (e.g., passphrase input) to prevent interference with the TUI.
	// Authentication must succeed non-interactively via SSH agent or unencrypted key.
	args = append(args, "-o", "BatchMode=yes")
	// Disable host key checking for parity with go+ssh.
	args = append(args, "-o", "StrictHostKeyChecking=accept-new")
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

	return args
}

// Dialer returns a proxy dialer for establishing connections within the cluster through SSH tunnels.
func (c *SSHCLIConnector) Dialer() (proxy.ContextDialer, error) {
	if c.config == (SSHConnectorConfig{}) {
		return nil, fmt.Errorf("SSH connector not configured")
	}

	return c, nil
}

// DialContext establishes a connection to the target address through an SSH tunnel using -W flag.
func (c *SSHCLIConnector) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" {
		return nil, fmt.Errorf("unsupported network type: %s", network)
	}
	if err := c.CheckTCPForwarding(ctx); err != nil {
		return nil, err
	}

	args := append(c.buildSSHArgs(), "-W", address)
	conn, err := commandconn.New(ctx, "ssh", args...)
	if err != nil {
		return nil, fmt.Errorf("SSH connection to '%s' for dialing '%s': %w", c.config.Destination(), address, err)
	}

	return conn, nil
}

// CheckTCPForwarding verifies that TCP forwarding is enabled on the remote SSH server. It probes the server once
// and caches the result for subsequent calls. Returns an error with actionable instructions if forwarding is disabled.
func (c *SSHCLIConnector) CheckTCPForwarding(ctx context.Context) error {
	c.fwdCheckOnce.Do(func() {
		// Probe TCP forwarding by requesting a forward to 127.0.0.1:1 (a port almost never in use).
		// If forwarding is disabled, sshd rejects the channel. The error message differs depending on
		// whether the connection goes through a ControlMaster mux or directly:
		//   - Multiplexed: "Session open refused by peer"
		//   - Direct: "administratively prohibited"
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		args := append(c.buildSSHArgs(), "-W", "127.0.0.1:1")
		probe := exec.CommandContext(probeCtx, "ssh", args...)
		output, _ := probe.CombinedOutput()
		outStr := string(output)
		if strings.Contains(outStr, "administratively prohibited") ||
			strings.Contains(outStr, "Session open refused by peer") {
			c.fwdCheckErr = fmt.Errorf(
				"SSH TCP forwarding appears to be disabled on '%s': ensure 'AllowTcpForwarding yes' is set "+
					"in /etc/ssh/sshd_config on the remote machine and restart sshd (sudo systemctl restart ssh)",
				c.config.Destination(),
			)
		}
	})
	return c.fwdCheckErr
}

func (c *SSHCLIConnector) Close() error {
	// Individual connections are managed by gRPC and closed when the gRPC connection closes.
	// The SSH control socket may persist for connection reuse across CLI invocations.
	return nil
}
