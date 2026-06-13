package connector

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	// fwdCheckOnce ensures the TCP forwarding check runs only once per connector.
	fwdCheckOnce sync.Once
	// fwdCheckErr caches the result of the TCP forwarding check.
	fwdCheckErr error

	// socksOnce lazily establishes the SOCKS tunnel used to multiplex DialContext connections when
	// ControlMaster is unavailable (see socksTunnelDialer).
	socksOnce   sync.Once
	socksDialer proxy.ContextDialer
	socksErr    error
	socksCancel context.CancelFunc
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
	// Windows OpenSSH does not support connection multiplexing (ControlMaster/ControlPath): the control
	// socket is a Unix domain socket and ssh fails with "getsockname failed: Not a socket". Return an
	// empty path to disable multiplexing so each connection is established independently.
	if runtime.GOOS == "windows" {
		return ""
	}

	// %C is expanded by `ssh` to a hash of user, local and remote hostnames, port, and the contents
	// of the ProxyJump option. This ensures that shared connections are uniquely identified.
	sockName := fmt.Sprintf("uc_control_%%C.sock")

	// Prefer XDG_RUNTIME_DIR if set and the directory exists, fall back to ~/.ssh if it exists.
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		// On WSL2 without systemd, XDG_RUNTIME_DIR may be set to /run/user/$UID that doesn't actually exist,
		// so existence must be verified before use: https://github.com/psviderski/uncloud/issues/319.
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return filepath.Join(dir, sockName)
		}
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
	probeArgs := append(c.buildSSHArgs(true), "true")
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
			dialArgs := append(c.buildSSHArgs(true), "uncloudd", "dial-stdio")
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
// include control socket settings for connection reuse if the path is configured and useControlMaster is true.
// The remote command is not included and should be appended by the caller.
func (c *SSHCLIConnector) buildSSHArgs(useControlMaster bool) []string {
	return append(c.sshOptions(useControlMaster), c.config.Destination())
}

// sshOptions returns the SSH command options (without the destination or any remote command). Callers that
// need to insert flags such as -D or -W before the destination compose them as
// append(sshOptions(...), flags..., Destination()).
func (c *SSHCLIConnector) sshOptions(useControlMaster bool) []string {
	var args []string

	// Add control socket options for connection reuse if available.
	if useControlMaster && c.controlSockPath != "" {
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

	c.fwdCheckOnce.Do(func() {
		c.fwdCheckErr = c.CheckTCPForwarding(ctx)
		if c.fwdCheckErr != nil {
			// Close the cached ControlMaster so the next call picks up the new sshd policy once the
			// user enables forwarding. Fresh context so close runs even if the parent already timed out.
			closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer closeCancel()
			c.CloseControlMaster(closeCtx)
		}
	})
	if c.fwdCheckErr != nil {
		return nil, c.fwdCheckErr
	}

	// When ControlMaster multiplexing is unavailable (e.g. on Windows), route the connection through a single
	// long-lived SOCKS tunnel instead of spawning a separate ssh process per dial. Without this, a burst of
	// parallel layer uploads to unregistry opens many simultaneous SSH connections, which the server rejects
	// during key exchange ("kex_exchange_identification: Connection reset"). The SOCKS tunnel multiplexes all
	// dials as port-forwarding channels over one SSH connection, which sshd does not rate-limit.
	if c.controlSockPath == "" {
		dialer, err := c.socksTunnelDialer(ctx)
		if err != nil {
			return nil, err
		}
		conn, err := dialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, fmt.Errorf("SSH connection to '%s' for dialing '%s': %w", c.config.Destination(), address, err)
		}
		return conn, nil
	}

	args := append(c.buildSSHArgs(true), "-W", address)
	conn, err := commandconn.New(ctx, "ssh", args...)
	if err != nil {
		return nil, fmt.Errorf("SSH connection to '%s' for dialing '%s': %w", c.config.Destination(), address, err)
	}

	return conn, nil
}

// socksTunnelDialer lazily starts a single `ssh -D` SOCKS proxy to the destination and returns a dialer that
// routes connections through it. The tunnel is started once per connector and reused by all subsequent dials,
// so any number of concurrent connections are multiplexed over a single SSH connection. The tunnel is torn
// down by Close. It uses the system ssh client, so it honours ~/.ssh/config host aliases, agents and keys
// exactly like the rest of the connector.
func (c *SSHCLIConnector) socksTunnelDialer(ctx context.Context) (proxy.ContextDialer, error) {
	c.socksOnce.Do(func() {
		port, err := freeLocalPort()
		if err != nil {
			c.socksErr = fmt.Errorf("reserve local port for SSH SOCKS tunnel: %w", err)
			return
		}
		socksAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

		// The tunnel must outlive the dial context that triggered it, so give it its own cancellable context
		// that is cancelled by Close.
		tunnelCtx, cancel := context.WithCancel(context.Background())
		args := append(c.sshOptions(false), "-N", "-D", socksAddr, c.config.Destination())
		cmd := exec.CommandContext(tunnelCtx, "ssh", args...)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			cancel()
			c.socksErr = fmt.Errorf("start SSH SOCKS tunnel to '%s': %w", c.config.Destination(), err)
			return
		}

		// Wait for the SOCKS proxy to start accepting connections (or for ssh to exit on error).
		if err := waitForSOCKSReady(ctx, socksAddr); err != nil {
			cancel()
			detail := strings.TrimSpace(stderr.String())
			if detail != "" {
				err = fmt.Errorf("%w: %s", err, detail)
			}
			c.socksErr = fmt.Errorf("establish SSH SOCKS tunnel to '%s': %w", c.config.Destination(), err)
			return
		}

		dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
		if err != nil {
			cancel()
			c.socksErr = fmt.Errorf("create SOCKS dialer: %w", err)
			return
		}
		ctxDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			cancel()
			c.socksErr = fmt.Errorf("SOCKS dialer does not support context dialing")
			return
		}

		c.socksCancel = cancel
		c.socksDialer = ctxDialer
	})

	return c.socksDialer, c.socksErr
}

// freeLocalPort reserves an available TCP port on the loopback interface by briefly listening and closing.
func freeLocalPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// waitForSOCKSReady polls addr until a TCP connection succeeds, the context is cancelled, or a 15s deadline
// is reached. It gives the ssh SOCKS proxy time to authenticate and bind its local port.
func waitForSOCKSReady(ctx context.Context, addr string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", addr)
		if err == nil {
			conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("SOCKS proxy did not become ready: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// CheckTCPForwarding returns an actionable error when the remote SSH server doesn't allow TCP forwarding.
func (c *SSHCLIConnector) CheckTCPForwarding(ctx context.Context) error {
	// Do not use ControlMaster because disabled forwarding and a refused port both surface as
	// "Session open refused by peer" over it and can't be told apart.
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Request forwarding to a port that is almost never in use (:1) so sshd rejects the channel if forwarding
	// is disabled or fails to connect otherwise.
	args := append(c.buildSSHArgs(false), "-W", "127.0.0.1:1")
	output, _ := exec.CommandContext(probeCtx, "ssh", args...).CombinedOutput()
	if strings.Contains(string(output), "administratively prohibited") {
		return fmt.Errorf("SSH TCP forwarding appears to be disabled on '%s': ensure 'AllowTcpForwarding yes' "+
			"is set in /etc/ssh/sshd_config on the remote machine and restart sshd (sudo systemctl restart ssh), "+
			"then retry",
			c.config.Destination())
	}

	return nil
}

// CloseControlMaster terminates the SSH ControlMaster process for this destination so the next connection starts
// a fresh SSH session. No-op if no master is running or the control socket is not configured. Errors are ignored.
func (c *SSHCLIConnector) CloseControlMaster(ctx context.Context) {
	if c.controlSockPath == "" {
		return
	}
	args := append(c.buildSSHArgs(true), "-O", "exit")
	_ = exec.CommandContext(ctx, "ssh", args...).Run()
}

func (c *SSHCLIConnector) Close() error {
	// Tear down the SOCKS tunnel if one was started (Windows/no-ControlMaster path).
	if c.socksCancel != nil {
		c.socksCancel()
	}
	// Individual connections are managed by gRPC and closed when the gRPC connection closes.
	// The SSH control socket may persist for connection reuse across CLI invocations.
	return nil
}
