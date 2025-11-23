package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"

	"github.com/moby/term"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
)

// ExecConfig contains options for executing a command in a container.
type ExecConfig struct {
	// Container ID to execute the command in.
	ContainerID string
	// Exec configuration.
	Options api.ExecOptions
}

// sendResizeRequest sends a terminal resize request to the exec stream.
func sendResizeRequest(stream pb.Docker_ExecContainerClient, size *term.Winsize) error {
	slog.Debug("sending resize request", "width", size.Width, "height", size.Height)
	return stream.Send(
		&pb.ExecContainerRequest{
			Payload: &pb.ExecContainerRequest_Resize{
				Resize: &pb.ResizeEvent{
					Height: uint32(size.Height),
					Width:  uint32(size.Width),
				},
			},
		})
}

// setupTerminal configures the terminal for interactive TTY sessions.
// It checks if stdin is a terminal, sets it to raw mode, and sets up resize handling.
// Returns a cleanup function to restore terminal state, or an error.
func setupTerminal(ctx context.Context, stream pb.Docker_ExecContainerClient) (func(), error) {
	inFd, isTerminal := term.GetFdInfo(os.Stdin)
	if !isTerminal {
		return nil, fmt.Errorf("stdin is not a terminal")
	}

	// Set terminal to raw mode
	oldState, err := term.SetRawTerminal(inFd)
	if err != nil {
		return nil, fmt.Errorf("set raw terminal: %w", err)
	}

	// Cleanup function
	restoreFunc := func() {
		_ = term.RestoreTerminal(inFd, oldState)
	}

	// Set up resize handling
	if err := handleTerminalResize(ctx, inFd, stream); err != nil {
		restoreFunc()
		return nil, err
	}

	return restoreFunc, nil
}

// handleTerminalResize sends initial window size and handles window resize signals for TTY sessions.
func handleTerminalResize(ctx context.Context, inFd uintptr, stream pb.Docker_ExecContainerClient) error {
	// Handle window resize signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, unix.SIGWINCH)

	// Send initial window size
	if size, err := term.GetWinsize(inFd); err == nil {
		_ = sendResizeRequest(stream, size)
	}

	go func() {
		defer signal.Stop(sigCh)
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigCh:
				size, err := term.GetWinsize(inFd)
				if err != nil {
					slog.Debug("get window size", "error", err)
					continue
				}
				if err = sendResizeRequest(stream, size); err != nil {
					slog.Debug("send resize request", "error", err)
				}
			}
		}
	}()

	return nil
}

// handleClientInputStream reads from stdin and sends data to the remote server.
// It also periodically checks for context cancellation to exit gracefully when e.g.
// the output stream is closed.
func handleClientInputStream(ctx context.Context, stream pb.Docker_ExecContainerClient, stdin io.Reader) error {
	slog.Debug("Input goroutine started")
	defer slog.Debug("Input goroutine exited")

	defer stream.CloseSend()

	// Channel to receive stdin data
	stdinCh := make(chan []byte)

	stdinErrCh := make(chan error, 1)

	// Read from stdin in a separate goroutine
	// Note: this goroutine may continue blocking on Read even after we exit from the function,
	// but that's OK - it will eventually unblock when data arrives or stdin closes.
	go func() {
		buf := make([]byte, 32*1024) // 32KB buffer
		for {
			n, err := stdin.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case stdinCh <- data:
				case <-ctx.Done():
					slog.Debug("stdin reader exiting due to context done")
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					slog.Debug("stdin reader: EOF received")
				} else {
					slog.Debug("stdin reader error", "error", err)
				}
				stdinErrCh <- err
				return
			}
		}
	}()

	// Send stdin data to the server or exit when context is cancelled
	for {
		select {
		case <-ctx.Done():
			return nil
		case data := <-stdinCh:
			if err := stream.Send(&pb.ExecContainerRequest{
				Payload: &pb.ExecContainerRequest_Stdin{Stdin: data},
			}); err != nil {
				return fmt.Errorf("send stdin: %w", err)
			}
		case err := <-stdinErrCh:
			if err != io.EOF {
				return fmt.Errorf("read stdin: %w", err)
			}
			return nil
		}
	}
}

// handleClientOutputStream receives output from the exec stream and writes to stdout/stderr.
// It also captures the exit code and signals completion via context cancellation.
func handleClientOutputStream(ctx context.Context, stream pb.Docker_ExecContainerClient, stdout, stderr io.Writer, exitCode *int) error {
	slog.Debug("Output goroutine started")
	defer slog.Debug("Output goroutine exited")

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			slog.Debug("output stream: EOF received")
			return nil
		}
		if err != nil {
			return fmt.Errorf("receive from stream: %w", err)
		}

		switch payload := resp.Payload.(type) {
		case *pb.ExecContainerResponse_ExecId:
			// This is sent first; we already processed it earlier, so just ignore duplicates.
		case *pb.ExecContainerResponse_Stdout:
			if _, err := stdout.Write(payload.Stdout); err != nil {
				return fmt.Errorf("write stdout: %w", err)
			}
		case *pb.ExecContainerResponse_Stderr:
			if _, err := stderr.Write(payload.Stderr); err != nil {
				return fmt.Errorf("write stderr: %w", err)
			}
		case *pb.ExecContainerResponse_ExitCode:
			slog.Debug("received exit code", "code", payload.ExitCode)
			*exitCode = int(payload.ExitCode)
			return nil
		}
	}
}

// ExecContainer executes a command in a running container with bidirectional streaming.
// TODO: This can be merged with pkg/client as it's an unnecessary logic split.
func (c *Client) ExecContainer(ctx context.Context, opts ExecConfig) (exitCode int, err error) {
	// Note: In non-interactive mode (without TTY), signals like SIGTERM/SIGINT will terminate the client
	// process without being forwarded to the remote container process. We could catch and forward these
	// signals (useful for long-running commands), but for example "kubectl exec" doesn't do this either (as of November 2025),
	// so we keep it simple for now.

	slog.Debug("starting ExecContainer", "containerID", opts.ContainerID, "options", opts.Options)

	// Initialize exit code to non-zero in case we have to return early
	exitCode = 1

	// Set up I/O streams - use custom streams if provided, otherwise default to os.Stdin/Stdout/Stderr
	stdin := io.Reader(os.Stdin)
	stdout := io.Writer(os.Stdout)
	stderr := io.Writer(os.Stderr)

	if opts.Options.Stdin != nil {
		stdin = opts.Options.Stdin
	}
	if opts.Options.Stdout != nil {
		stdout = opts.Options.Stdout
	}
	if opts.Options.Stderr != nil {
		stderr = opts.Options.Stderr
	}

	// Marshal the exec config
	configBytes, err := json.Marshal(opts.Options)
	if err != nil {
		return -1, fmt.Errorf("marshal exec config: %w", err)
	}

	// Create the bidirectional stream
	stream, err := c.GRPCClient.ExecContainer(ctx)
	if err != nil {
		return -1, fmt.Errorf("create exec stream: %w", err)
	}

	// Send the initial configuration
	if err := stream.Send(&pb.ExecContainerRequest{
		Payload: &pb.ExecContainerRequest_Config{
			Config: &pb.ExecConfig{
				ContainerId: opts.ContainerID,
				Options:     configBytes,
			},
		},
	}); err != nil {
		return -1, fmt.Errorf("send exec config: %w", err)
	}

	// Receive the exec ID
	resp, err := stream.Recv()
	if err != nil {
		return -1, fmt.Errorf("receive exec ID: %w", err)
	}
	execID := resp.GetExecId()
	if execID == "" {
		return -1, fmt.Errorf("expected exec ID in first response")
	}

	errGroup, ctx := errgroup.WithContext(ctx)

	// Create cancellable context for goroutine coordination
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle terminal setup for interactive sessions
	if opts.Options.AttachStdin && opts.Options.Tty {
		restoreTerminal, err := setupTerminal(ctx, stream)
		if err != nil {
			return -1, fmt.Errorf("setup terminal: %w", err)
		}
		if restoreTerminal != nil {
			defer restoreTerminal()
		}
	}

	// Handle stdin stream if needed
	if opts.Options.AttachStdin {
		errGroup.Go(func() error {
			return handleClientInputStream(ctx, stream, stdin)
		})
	} else {
		// Close send direction immediately if not attaching stdin
		stream.CloseSend()
	}

	// Handle output streams (stdout/stderr)
	errGroup.Go(func() error {
		defer cancel()
		return handleClientOutputStream(ctx, stream, stdout, stderr, &exitCode)
	})

	err = errGroup.Wait()

	if err == nil && opts.Options.Detach {
		return 0, nil
	}

	return exitCode, err
}
