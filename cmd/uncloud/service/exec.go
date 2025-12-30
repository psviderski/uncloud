package service

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/cli/cli/streams"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type execCliOptions struct {
	detach      bool
	interactive bool
	noTty       bool
	containerId string
}

var DEFAULT_COMMAND = []string{"sh", "-c", "command -v bash >/dev/null 2>&1 && exec bash || exec sh"}

func NewExecCommand(groupID string) *cobra.Command {
	opts := execCliOptions{}

	execCmd := &cobra.Command{
		Use:   "exec [OPTIONS] SERVICE [COMMAND ARGS...]",
		Short: "Execute a command in a running service container.",
		Long: `Execute a command (interactive shell by default) in a running container within a service.
If the service has multiple replicas and no container ID is specified, the command will be executed in a random container.
	`,
		Example: `
  # Start an interactive shell ("bash" or "sh" will be tried by default)
  uc exec web-service

  # Start an interactive shell with explicit command
  uc exec web-service /bin/zsh

  # List files in the specific container of the service; --container accepts full ID or a (unique) prefix
  uc exec --container d792e web-service ls -la

  # Pipe input to a command inside the service container
  cat backup.sql | uc exec -T db-service psql -U postgres mydb

  # Run a task in the background (detached mode)
  uc exec -d web-service /scripts/cleanup.sh`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			serviceName := args[0]
			command := args[1:]
			if len(command) == 0 {
				command = DEFAULT_COMMAND
			}
			return runExec(cmd.Context(), uncli, serviceName, command, opts)
		},
		GroupID: groupID,
	}

	execCmd.Flags().BoolVarP(&opts.detach, "detach", "d", false, "Detached mode: run command in the background")

	execCmd.Flags().BoolVarP(&opts.noTty, "no-tty", "T", false,
		"Disable pseudo-TTY allocation. By default 'uc exec' allocates a TTY when connected to a terminal.")

	// Keep "-i" and "-t" flags hidden for compatibility with docker exec
	execCmd.Flags().BoolVarP(&opts.interactive, "interactive", "i", true, "Keep STDIN open even if not attached")
	execCmd.Flags().MarkHidden("interactive")

	execCmd.Flags().BoolP("tty", "t", false, "Allocate a pseudo-TTY")
	execCmd.Flags().MarkHidden("tty")

	// Common flags
	execCmd.Flags().StringVar(&opts.containerId, "container", "",
		"ID of the container to exec into. Accepts full ID or a unique prefix "+
			"(default is the random container of the service)")

	// This tells Cobra that all flags must come before positional arguments, so that
	// commands with their own flags can be handled correctly.
	execCmd.Flags().SetInterspersed(false)

	return execCmd
}

func runExec(ctx context.Context, uncli *cli.CLI, serviceName string, command []string, opts execCliOptions) error {
	// Disable TTY allocation if not connected to a terminal
	if !cli.IsStdoutTerminal() {
		opts.noTty = true
	}

	if !opts.detach {
		// Check if we're trying to attach to a TTY from a non-TTY client, e.g.
		// when doing an 'cmd | uc exec ...'
		stdin := streams.NewIn(os.Stdin)
		// TODO: this logic/behavior mirrors docker-compose, but we can be smarter about it and detect TTY dynamically
		if err := stdin.CheckTty(opts.interactive, !opts.noTty); err != nil {
			return fmt.Errorf("check TTY: %w; use -T option to disable TTY allocation", err)
		}
	}

	client, err := uncli.ConnectClusterWithOptions(ctx, cli.ConnectOptions{
		ShowProgress: false,
	})
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	execConfig := api.ExecOptions{
		Command:     command,
		AttachStdin: opts.interactive,
		Tty:         !opts.noTty,
		Detach:      opts.detach,
	}

	if !opts.detach {
		execConfig.AttachStdout = true
		execConfig.AttachStderr = true
	}

	exitCode, err := client.ExecContainer(ctx, serviceName, opts.containerId, execConfig)
	if err != nil {
		return fmt.Errorf("exec container: %w", err)
	}

	// For non-detached mode, exit with the same code as the executed command
	if !opts.detach {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}

	return nil
}
