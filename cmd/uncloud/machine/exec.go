package machine

import (
	"context"
	"fmt"
	"os"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

func NewExecCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec MACHINE COMMAND [ARGS...]",
		Short: "Execute a command on a machine.",
		Long: `Execute a command on a machine in the cluster.

The command is executed directly on the target machine. Shell features such as pipes,
redirection, and variable expansion require an explicit shell command like "sh -c".`,
		Example: `  # Print the machine hostname.
  uc machine exec machine1 hostname

  # Check a systemd service.
  uc machine exec machine1 systemctl is-active docker

  # Run shell syntax explicitly.
  uc machine exec machine1 -- sh -c 'systemctl is-active docker && docker ps'`,
		Args: validateExecArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			machineNameOrID, command := normalizeExecArgs(args)
			return runExec(cmd.Context(), uncli, machineNameOrID, command)
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return completion.Machines(cmd.Context(), uncli, args, toComplete)
		},
	}

	// This tells Cobra that all flags must come before positional arguments, so commands
	// with their own flags can be handled correctly after "--".
	cmd.Flags().SetInterspersed(false)

	return cmd
}

func validateExecArgs(_ *cobra.Command, args []string) error {
	machineNameOrID, command := normalizeExecArgs(args)
	if machineNameOrID == "" {
		return fmt.Errorf("machine is required")
	}
	if len(command) == 0 {
		return fmt.Errorf("command is required")
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

func runExec(ctx context.Context, uncli *cli.CLI, machineNameOrID string, command []string) error {
	client, err := uncli.ConnectClusterWithOptions(ctx, cli.ConnectOptions{
		ShowProgress: false,
	})
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	exitCode, err := client.ExecMachine(ctx, machineNameOrID, api.MachineExecOptions{
		Command: command,
	})
	if err != nil {
		return fmt.Errorf("exec command on machine: %w", err)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}
