package service

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

type execOptions struct {
	detach      bool
	interactive bool
	tty         bool
	context     string
}

func NewExecCommand() *cobra.Command {
	opts := execOptions{}

	cmd := &cobra.Command{
		Use:   "exec [OPTIONS] SERVICE COMMAND [ARGS...]",
		Short: "Execute a command in a running container",
		Long: `Execute a command in a running container within a service.

If the service has multiple replicas, the command will be executed in the first container.

Examples:
  # List files in a container
  uc exec web-service ls -la

  # Start an interactive shell
  uc exec -it web-service bash

  # Run a background task
  uc exec -d web-service /scripts/cleanup.sh`,
		Args: cobra.MinimumNArgs(2),
		// DisableFlagParsing would disable all flag parsing, but we want to parse our own flags.
		// Instead, we'll use FParseErrWhitelist to ignore unknown flags which will be passed to the command.
		FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return runExec(cmd.Context(), uncli, args[0], args[1:], opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.detach, "detach", "d", false,
		"Detached mode: run command in the background")
	cmd.Flags().BoolVarP(&opts.interactive, "interactive", "i", false,
		"Keep STDIN open even if not attached")
	cmd.Flags().BoolVarP(&opts.tty, "tty", "t", false,
		"Allocate a pseudo-TTY")
	cmd.Flags().StringVarP(&opts.context, "context", "c", "",
		"Name of the cluster context. (default is the current context)")

	// This tells Cobra that all flags must come before positional arguments
	cmd.Flags().SetInterspersed(false)

	return cmd
}

func runExec(ctx context.Context, uncli *cli.CLI, serviceName string, command []string, opts execOptions) error {
	c, err := uncli.ConnectCluster(ctx, opts.context)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer c.Close()

	// Build the exec configuration
	execConfig := container.ExecOptions{
		Cmd:          command,
		AttachStdin:  opts.interactive,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          opts.tty,
		Detach:       opts.detach,
	}

	// Execute the command in the first container of the service
	exitCode, err := c.ExecContainer(ctx, serviceName, "", execConfig)
	if err != nil {
		return err
	}

	// For non-detached mode, exit with the same code as the executed command
	if !opts.detach {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}

	return nil
}
