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

func installCmd(user string) string {
	sudoPrefix := "sudo"
	// Add the SSH user (non-root) to the uncloud group to allow access to the Uncloud daemon unix socket.
	env := "UNCLOUD_GROUP_ADD_USER=" + user

	curlBashCmd := fmt.Sprintf(
		"curl -fsSL %s | %s %s bash", sshexec.Quote(installScriptURL), sudoPrefix, sshexec.Quote(env),
	)

	if user == "root" {
		curlBashCmd = fmt.Sprintf(
			"curl -fsSL %s | bash", sshexec.Quote(installScriptURL),
		)
	}

	return curlBashCmd
}

// provisionMachine provisions the remote machine by downloading the Uncloud install script from GitHub and running it.
func provisionMachine(ctx context.Context, exec sshexec.Executor) error {
	user, err := exec.Run(ctx, "whoami")
	if err != nil {
		return fmt.Errorf("run whoami: %w", err)
	}

	installCmd := installCmd(user)

	fmt.Println("Downloading Uncloud install script:", installScriptURL)

	cmd := sshexec.QuoteCommand("bash", "-c", "set -o pipefail; "+installCmd)
	if err = exec.Stream(ctx, cmd, os.Stdout, os.Stderr); err != nil {
		return fmt.Errorf("download and run install script: %w", err)
	}
	return nil
}
