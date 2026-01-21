package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/charmbracelet/huh"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/sshexec"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	// TODO: support pinning the script version to the CLI version.
	installScriptURL = "https://raw.githubusercontent.com/psviderski/uncloud/refs/heads/main/scripts/install.sh"
	rootUser         = "root"
)

type RemoteMachine struct {
	User      string
	Host      string
	Port      int
	KeyPath   string
	UseSSHCLI bool // indicates ssh+cli:// should be used
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

func promptResetMachine() error {
	if !IsStdinTerminal() {
		return errors.New("the remote machine is already initialised as a cluster member; " +
			"cannot ask to confirm reset in non-interactive mode, " +
			"use --yes flag or set UNCLOUD_AUTO_CONFIRM=true to auto-confirm")
	}

	var confirm bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(
					"The remote machine is already initialised as a cluster member. Do you want to reset it first?\n" +
						"This will:\n" +
						"- Remove all service containers from the machine\n" +
						"- Reset the machine to the uninitialised state",
				).
				Affirmative("Yes!").
				Negative("No").
				Value(&confirm),
		),
	).WithAccessible(true)
	if err := form.Run(); err != nil {
		return fmt.Errorf("prompt user to confirm: %w", err)
	}

	if !confirm {
		return fmt.Errorf("remote machine is already initialised as a cluster member")
	}

	return nil
}

func resetAndWaitMachine(ctx context.Context, machineClient pb.MachineClient) error {
	if _, err := machineClient.Reset(ctx, &pb.ResetRequest{}); err != nil {
		return fmt.Errorf("reset remote machine: %w. You can also manually run 'uncloud-uninstall' "+
			"on the remote machine to fully uninstall Uncloud from it", err)
	}

	fmt.Println("Resetting the remote machine...")
	if err := waitMachineReady(ctx, machineClient, 1*time.Minute); err != nil {
		return fmt.Errorf("wait for machine to be ready after reset: %w", err)
	}

	return nil
}

// waitMachineReady waits for the machine to be ready to serve requests.
func waitMachineReady(ctx context.Context, machineClient pb.MachineClient, timeout time.Duration) error {
	boff := backoff.WithContext(backoff.NewExponentialBackOff(
		backoff.WithMaxInterval(1*time.Second),
		backoff.WithMaxElapsedTime(timeout),
	), ctx)

	inspect := func() error {
		_, err := machineClient.Inspect(ctx, &emptypb.Empty{})
		if err != nil {
			return fmt.Errorf("inspect machine: %w", err)
		}
		return nil
	}
	return backoff.Retry(inspect, boff)
}
