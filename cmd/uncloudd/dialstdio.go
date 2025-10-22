package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/psviderski/uncloud/internal/machine"
	"github.com/spf13/cobra"
)

func newDialStdioCommand() *cobra.Command {
	var socketPath string

	cmd := &cobra.Command{
		Use:    "dial-stdio",
		Short:  "Proxy stdin/stdout to the Uncloud API socket",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDialStdio(cmd.Context(), socketPath, os.Stdin, os.Stdout)
		},
	}

	cmd.Flags().StringVar(&socketPath, "socket", machine.DefaultUncloudSockPath,
		"Path to the Uncloud API socket")

	return cmd
}

func runDialStdio(ctx context.Context, socketPath string, stdin io.Reader, stdout io.Writer) error {
	// Connect to the unix socket.
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("connect to socket %q: %w", socketPath, err)
	}
	defer conn.Close()

	// Copy data bidirectionally between stdin/stdout and the socket.
	errCh := make(chan error, 2)

	// Copy from stdin to socket.
	go func() {
		_, err := io.Copy(conn, stdin)
		errCh <- err
	}()

	// Copy from socket to stdout.
	go func() {
		_, err := io.Copy(stdout, conn)
		errCh <- err
	}()

	// Wait for either direction to complete or context to be canceled.
	select {
	case err := <-errCh:
		// One direction finished (possibly with an error).
		// This is normal when the connection closes.
		if err != nil && err != io.EOF {
			return fmt.Errorf("copy error: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
