package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/psviderski/uncloud/internal/sshexec"
)

const (
	// TODO: support pinning the script version to the CLI version.
	installScriptURL = "https://raw.githubusercontent.com/psviderski/uncloud/refs/heads/main/scripts/install.sh"
	rootUser         = "root"
)

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
	if user != rootUser {
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

	if user != rootUser {
		// 'sudo -n' is not used because it fails with 'sudo: a password is required' when the user has no password
		// in /etc/shadow even though it may have valid sudo access.
		out, err := exec.Run(ctx, "sudo true")
		if err != nil {
			if strings.Contains(out, "password is required") {
				return fmt.Errorf(
					"user '%[1]s' requires a password for sudo, but Uncloud needs passwordless sudo or root access "+
						"to install and configure the uncloudd daemon on the remote machine.\n\n"+
						"Possible solutions:\n"+
						"1. Use root user or a user with passwordless sudo instead.\n"+
						"2. Configure passwordless sudo for the user '%[1]s' by running on the remote machine:\n"+
						"   echo '%[1]s ALL=(ALL) NOPASSWD:ALL' | sudo tee /etc/sudoers.d/%[1]s",
					user)
			}
			return fmt.Errorf("sudo command failed for user '%s': %w. "+
				"Please ensure the user has sudo privileges or use root user instead", user, err)
		}
	}

	cmd := installCmd(user, version)

	fmt.Println("Downloading Uncloud install script:", installScriptURL)

	cmd = sshexec.QuoteCommand("bash", "-c", "set -o pipefail; "+cmd)
	if err = exec.Stream(ctx, cmd, os.Stdout, os.Stderr); err != nil {
		return fmt.Errorf("download and run install script: %w", err)
	}
	return nil
}
