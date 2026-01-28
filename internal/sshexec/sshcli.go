package sshexec

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
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

// TODO: Refactor and reuse this with buildDialArgs and buildSSHArgs from
// SSHCLI Connector.
func (r *SSHCLIRemote) buildSSHArgs() []string {
	args := []string{"-o", "ConnectTimeout=5"}

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
	args = append(args, dst)

	return args
}

func (r *SSHCLIRemote) Run(ctx context.Context, cmd string) (string, error) {
	args := r.buildSSHArgs()
	args = append(args, cmd)

	execCmd := exec.CommandContext(ctx, "ssh", args...)
	output, err := execCmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output)),
			fmt.Errorf("run command on remote host: %w: %s", err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func (r *SSHCLIRemote) Stream(ctx context.Context, cmd string, stdout, stderr io.Writer) error {
	args := r.buildSSHArgs()
	args = append(args, cmd)

	execCmd := exec.CommandContext(ctx, "ssh", args...)
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr

	return execCmd.Run()
}

// no-op as there is no persistent connection.
func (r *SSHCLIRemote) Close() error {
	return nil
}
