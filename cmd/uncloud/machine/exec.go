package machine

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/cli/cli/streams"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type execCliOptions struct {
	interactive bool
	noTty       bool
}

var defaultExecCommand = []string{"sh", "-c", "command -v bash >/dev/null 2>&1 && exec bash || exec sh"}

func NewExecCommand() *cobra.Command {
	opts := execCliOptions{}

	cmd := &cobra.Command{
		Use:   "exec [OPTIONS] MACHINE [COMMAND ARGS...]",
		Short: "Execute a command on a machine.",
		Long: `Execute a command, or an interactive shell by default, on a machine in the cluster.

The command is executed directly on the target machine. Shell features such as pipes,
redirection, and variable expansion require an explicit shell command like "sh -c".`,
		Example: `  # Start an interactive shell.
  uc machine exec machine1

  # Start an interactive shell with explicit command.
  uc machine exec machine1 /bin/bash

  # Print the machine hostname.
  uc machine exec machine1 hostname

  # Pipe input to a command.
  cat script.sh | uc machine exec -T machine1 sh

  # Check a systemd service.
  uc machine exec machine1 systemctl is-active docker

  # Run shell syntax explicitly.
  uc machine exec machine1 -- sh -c 'systemctl is-active docker && docker ps'`,
		Args: validateExecArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			machineNameOrID, command := normalizeExecArgs(args)
			if len(command) == 0 {
				command = defaultExecCommand
			}
			return runExec(cmd.Context(), uncli, machineNameOrID, command, opts)
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return completion.Machines(cmd.Context(), uncli, args, toComplete)
		},
	}

	cmd.Flags().BoolVarP(&opts.noTty, "no-tty", "T", false,
		"Disable pseudo-TTY allocation. By default 'uc machine exec' allocates a TTY when connected to a terminal.")

	// Keep "-i" and "-t" flags hidden for compatibility with docker exec.
	cmd.Flags().BoolVarP(&opts.interactive, "interactive", "i", true, "Keep STDIN open even if not attached")
	cmd.Flags().MarkHidden("interactive")

	cmd.Flags().BoolP("tty", "t", false, "Allocate a pseudo-TTY")
	cmd.Flags().MarkHidden("tty")

	// This tells Cobra that all flags must come before positional arguments, so commands
	// with their own flags can be handled correctly after "--".
	cmd.Flags().SetInterspersed(false)

	return cmd
}

func validateExecArgs(_ *cobra.Command, args []string) error {
	machineNameOrID, _ := normalizeExecArgs(args)
	if machineNameOrID == "" {
		return fmt.Errorf("machine is required")
	}
	return nil
}

func normalizeExecArgs(args []string) (machineNameOrID string, command []string) {
	if len(args) == 0 {
		return "", nil
	}

	machineNameOrID = args[0]
	command = args[1:]
	if len(command) > 0 && command[0] == "--" {
		command = command[1:]
	}
	return machineNameOrID, command
}

func runExec(ctx context.Context, uncli *cli.CLI, machineNameOrID string, command []string, opts execCliOptions) error {
	if !tui.IsStdoutTerminal() {
		opts.noTty = true
	}

	stdin := streams.NewIn(os.Stdin)
	if err := stdin.CheckTty(opts.interactive, !opts.noTty); err != nil {
		return fmt.Errorf("check TTY: %w; use -T option to disable TTY allocation", err)
	}

	client, err := uncli.ConnectClusterWithOptions(ctx, cli.ConnectOptions{
		ShowProgress: false,
	})
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	exitCode, err := client.ExecMachine(ctx, machineNameOrID, api.MachineExecOptions{
		Command:     command,
		AttachStdin: opts.interactive,
		Tty:         !opts.noTty,
	})
	if err != nil {
		return fmt.Errorf("exec command on machine: %w", err)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}
