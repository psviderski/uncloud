package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/psviderski/uncloud/internal/sshexec"
)

// TODO: support pinning the script version to the CLI version.
const installScriptURL = "https://raw.githubusercontent.com/psviderski/uncloud/refs/heads/main/scripts/install.sh"

type RemoteMachine struct {
	User    string
	Host    string
	Port    int
	KeyPath string
}

func installCmd(user string, version string) string {
	sudoPrefix := ""
	var env []string

	// Add the SSH user (non-root) to the uncloud group to allow access to the Uncloud daemon unix socket.
	if user != "root" {
		sudoPrefix = "sudo"
		env = append(env, "UNCLOUD_GROUP_ADD_USER="+sshexec.Quote(user))
	}
	if version != "" {
		env = append(env, "UNCLOUD_VERSION="+sshexec.Quote(version))
	}

	envCmd := strings.Join(env, " ")
	curlBashCmd := fmt.Sprintf("curl -fsSL %s | %s %s bash", sshexec.Quote(installScriptURL), sudoPrefix, envCmd)

	return curlBashCmd
}

// provisionMachine provisions the remote machine by downloading the Uncloud install script from GitHub and running it.
// If version is specified, it will be passed to the install script as UNCLOUD_VERSION environment variable.
func provisionMachine(ctx context.Context, exec sshexec.Executor, version string) error {
	user, err := exec.Run(ctx, "whoami")
	if err != nil {
		return fmt.Errorf("run whoami: %w", err)
	}

	cmd := installCmd(user, version)

	fmt.Println("Downloading Uncloud install script:", installScriptURL)

	cmd = sshexec.QuoteCommand("bash", "-c", "set -o pipefail; "+cmd)
	if err = exec.Stream(ctx, cmd, os.Stdout, os.Stderr); err != nil {
		return fmt.Errorf("download and run install script: %w", err)
	}
	return nil
}
