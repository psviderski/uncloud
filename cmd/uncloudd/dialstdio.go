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

// halfReadCloser is the read side of a half-duplex connection.
type halfReadCloser interface {
	io.Reader
	CloseRead() error
}

// halfWriteCloser is the write side of a half-duplex connection.
type halfWriteCloser interface {
	io.Writer
	CloseWrite() error
}

// halfReadCloserWrapper wraps an io.ReadCloser to implement halfReadCloser.
type halfReadCloserWrapper struct {
	io.ReadCloser
}

func (x *halfReadCloserWrapper) CloseRead() error {
	return x.Close()
}

// halfWriteCloserWrapper wraps an io.WriteCloser to implement halfWriteCloser.
type halfWriteCloserWrapper struct {
	io.WriteCloser
}

func (x *halfWriteCloserWrapper) CloseWrite() error {
	return x.Close()
}

func runDialStdio(ctx context.Context, socketPath string, stdin io.Reader, stdout io.Writer) error {
	// Connect to the unix socket.
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("connect to socket %q: %w", socketPath, err)
	}
	defer conn.Close()

	// Wrap stdin/stdout to support half-closing.
	var stdinCloser halfReadCloser
	if c, ok := stdin.(halfReadCloser); ok {
		stdinCloser = c
	} else if c, ok := stdin.(io.ReadCloser); ok {
		stdinCloser = &halfReadCloserWrapper{c}
	}

	var stdoutCloser halfWriteCloser
	if c, ok := stdout.(halfWriteCloser); ok {
		stdoutCloser = c
	} else if c, ok := stdout.(io.WriteCloser); ok {
		stdoutCloser = &halfWriteCloserWrapper{c}
	}

	// Copy data bidirectionally between stdin/stdout and the socket.
	stdin2socket := make(chan error, 1)
	socket2stdout := make(chan error, 1)

	// Copy from stdin to socket.
	go func() {
		_, err := io.Copy(conn, stdin)
		stdin2socket <- err
		// Close write side of connection after stdin is done.
		if unixConn, ok := conn.(*net.UnixConn); ok {
			unixConn.CloseWrite()
		}
		if stdinCloser != nil {
			stdinCloser.CloseRead()
		}
	}()

	// Copy from socket to stdout.
	go func() {
		_, err := io.Copy(stdout, conn)
		socket2stdout <- err
		// Close read side of connection after socket is done sending.
		if unixConn, ok := conn.(*net.UnixConn); ok {
			unixConn.CloseRead()
		}
		if stdoutCloser != nil {
			stdoutCloser.CloseWrite()
		}
	}()

	select {
	case err = <-stdin2socket:
		if err != nil {
			return err
		}
	case err = <-socket2stdout:
		// return immediately, matching Docker's approach
		// (stdin is never closed when TTY)
	}
	return err
}
