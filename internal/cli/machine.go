package cli

import (
	"context"
	"fmt"
	"os"
	"uncloud/internal/sshexec"
)

// TODO: support pinning the script version to the CLI version.
const installScriptURL = "https://raw.githubusercontent.com/psviderski/uncloud/refs/heads/main/scripts/install.sh"

type RemoteMachine struct {
	User    string
	Host    string
	Port    int
	KeyPath string
}

// provisionMachine provisions the remote machine by downloading the Uncloud install script from GitHub and running it.
func provisionMachine(ctx context.Context, exec sshexec.Executor) error {
	// TODO: Check if the machine is already provisioned and ask the user to reset it first.

	user, err := exec.Run(ctx, "whoami")
	if err != nil {
		return fmt.Errorf("run whoami: %w", err)
	}
	sudoPrefix, env := "", ""
	if user != "root" {
		sudoPrefix = "sudo"
		// Add the SSH user (non-root) to the uncloud group to allow access to the Uncloud daemon unix socket.
		env = "UNCLOUD_GROUP_ADD_USER=" + user
	}

	fmt.Println("Downloading Uncloud install script:", installScriptURL)
	curlBashCmd := fmt.Sprintf(
		"curl -fsSL %s | %s %s bash", sshexec.Quote(installScriptURL), sudoPrefix, sshexec.Quote(env),
	)
	cmd := sshexec.QuoteCommand("bash", "-c", "set -o pipefail; "+curlBashCmd)
	// TODO: figure out why sometimes the output of `docker version` within the script is intermixed with other
	//  script output. Note, that the same behavior is observed when using exec.Run but not when running the script
	//  using ssh CLI. Requesting a pseudo-terminal may help fix this issue:
	//		session.RequestPty("xterm", 40, 80, ssh.TerminalModes{})
	if err = exec.Stream(ctx, cmd, os.Stdout, os.Stderr); err != nil {
		return fmt.Errorf("download and run install script: %w", err)
	}
	return nil
}
