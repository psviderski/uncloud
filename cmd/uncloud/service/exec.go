package service

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/cli/cli/streams"
	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

type execOptions struct {
	detach      bool
	interactive bool
	noTty       bool
	context     string
}

var DEFAULT_COMMAND = []string{"sh", "-c", "command -v bash >/dev/null 2>&1 && exec bash || exec sh"}

func NewExecCommand() *cobra.Command {
	opts := execOptions{}

	execCmd := &cobra.Command{
		Use:   "exec [OPTIONS] SERVICE [COMMAND ARGS...]",
		Short: "Execute a command in a running service container",
		Long: `Execute a command (interactive shell by default) in a running container within a service.
(FIXME) If the service has multiple replicas, the command will be executed in the first container.
	`,
		Example: `
  # Start an interactive shell ("bash" or "sh" will be tried by default)
  uc exec web-service

  # Start an interactive shell with explicit command
  uc exec web-service /bin/zsh

  # List files in a container
  uc exec web-service ls -la

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
	}

	execCmd.Flags().BoolVarP(&opts.detach, "detach", "d", false, "Detached mode: run command in the background")

	execCmd.Flags().BoolVarP(&opts.noTty, "no-tty", "T", !cli.IsStdoutTerminal(),
		"Disable pseudo-TTY allocation. By default 'uc exec' allocates a TTY when connected to a terminal.")

	// Keep "-i" and "-t" flags hidden for compatibility with docker exec
	execCmd.Flags().BoolVarP(&opts.interactive, "interactive", "i", true, "Keep STDIN open even if not attached")
	execCmd.Flags().MarkHidden("interactive")

	execCmd.Flags().BoolP("tty", "t", false, "Allocate a pseudo-TTY")
	execCmd.Flags().MarkHidden("tty")

	execCmd.Flags().StringVarP(&opts.context, "context", "c", "",
		"Name of the cluster context. (default is the current context)")

	// This tells Cobra that all flags must come before positional arguments, so that
	// commands with their own flags can be handled correctly.
	execCmd.Flags().SetInterspersed(false)

	return execCmd
}

func runExec(ctx context.Context, uncli *cli.CLI, serviceName string, command []string, opts execOptions) error {
	if !opts.detach {
		// Check if we're trying to attach to a TTY from a non-TTY client, e.g.
		// when doing an 'cmd | uc exec ...'
		stdin := streams.NewIn(os.Stdin)
		if err := stdin.CheckTty(opts.interactive, !opts.noTty); err != nil {
			return fmt.Errorf("check TTY: %w; use -T option to disable TTY allocation", err)
		}
	}

	c, err := uncli.ConnectClusterWithOptions(ctx, opts.context, cli.ConnectOptions{
		ShowProgress: false,
	})
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer c.Close()

	execOpts := container.ExecOptions{
		Cmd:         command,
		AttachStdin: opts.interactive,
		Tty:         !opts.noTty,
	}

	if !opts.detach {
		execOpts.AttachStdout = true
		execOpts.AttachStderr = true
	}

	// Execute the command in the first container of the service
	exitCode, err := c.ExecContainer(ctx, serviceName, "", execOpts)
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
