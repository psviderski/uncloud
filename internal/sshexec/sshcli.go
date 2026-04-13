package sshexec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type SSHCLIRemote struct {
	user    string
	host    string
	port    int
	keyPath string
}

func NewSSHCLIRemote(user, host string, port int, keyPath string) *SSHCLIRemote {
	return &SSHCLIRemote{
		user:    user,
		host:    host,
		port:    port,
		keyPath: keyPath,
	}
}

// newSSHCommand creates an exec.Cmd for ssh that sends SIGINT on context cancellation, giving the
// remote process a chance to exit gracefully before being killed.
func (r *SSHCLIRemote) newSSHCommand(ctx context.Context, cmd string) *exec.Cmd {
	args := []string{
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=accept-new",
		// Disable pseudo-terminal allocation to prevent SSH from executing as a login shell.
		"-T",
	}

	if r.port != 0 {
		args = append(args, "-p", strconv.Itoa(r.port))
	}
	if r.keyPath != "" {
		args = append(args, "-i", r.keyPath)
	}

	dst := r.host
	if r.user != "" {
		dst = fmt.Sprintf("%s@%s", r.user, dst)
	}
	args = append(args, dst, cmd)

	execCmd := exec.CommandContext(ctx, "ssh", args...)
	execCmd.Cancel = func() error {
		return execCmd.Process.Signal(os.Interrupt)
	}
	execCmd.WaitDelay = 5 * time.Second

	return execCmd
}

func (r *SSHCLIRemote) Run(ctx context.Context, cmd string) (string, error) {
	var stdout, stderr bytes.Buffer
	err := r.Stream(ctx, cmd, &stdout, &stderr)
	out := strings.TrimSpace(stdout.String())
	if err != nil {
		return out, fmt.Errorf("%w: %s", err, stderr.String())
	}
	return out, nil
}

func (r *SSHCLIRemote) Stream(ctx context.Context, cmd string, stdout, stderr io.Writer) error {
	sshCmd := r.newSSHCommand(ctx, cmd)
	sshCmd.Stdout = stdout
	sshCmd.Stderr = stderr

	if err := sshCmd.Run(); err != nil {
		return fmt.Errorf("run command on remote host: %w", err)
	}
	return nil
}

// Close is no-op as there is no persistent connection.
func (r *SSHCLIRemote) Close() error {
	return nil
}
