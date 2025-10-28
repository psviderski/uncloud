package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"

	"github.com/moby/term"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
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
				_ = sendResizeRequest(stream, size)
			}
		}
	}()

	return nil
}

// ExecContainer executes a command in a running container with bidirectional streaming.
// TODO: This can be merged with pkg/client as it's an unnecessary logic split.
func (c *Client) ExecContainer(ctx context.Context, opts ExecConfig) (int, error) {
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

	var exitCode int
	errCh := make(chan error, 2) // Max 2 concurrent goroutines (stdin + output)

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

	var wg sync.WaitGroup

	// Goroutine to read from stdin and send to stream
	if opts.Options.AttachStdin {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer stream.CloseSend()

			// Channel to receive stdin data
			stdinCh := make(chan []byte)
			stdinErrCh := make(chan error, 1)

			// Read from stdin in a separate goroutine
			// Note: this goroutine may continue blocking on Read even after we exit,
			// but that's OK - it will eventually unblock when data arrives or stdin closes
			go func() {
				buf := make([]byte, 32*1024) // 32KB buffer
				for {
					n, err := os.Stdin.Read(buf)
					if n > 0 {
						data := make([]byte, n)
						copy(data, buf[:n])
						select {
						case stdinCh <- data:
						case <-ctx.Done():
							return
						}
					}
					if err != nil {
						stdinErrCh <- err
						return
					}
				}
			}()

			// Send stdin data or exit when context is cancelled
			for {
				select {
				case <-ctx.Done():
					return
				case data := <-stdinCh:
					if err := stream.Send(&pb.ExecContainerRequest{
						Payload: &pb.ExecContainerRequest_Stdin{Stdin: data},
					}); err != nil {
						return
					}
				case err := <-stdinErrCh:
					if err != io.EOF {
						errCh <- fmt.Errorf("read stdin: %w", err)
					}
					return
				}
			}
		}()
	}
	// Note: We don't call CloseSend() for non-interactive mode.
	// The stream will be closed when this function returns.
	// Calling CloseSend() early can cause the server's context to be canceled prematurely.

	// Goroutine to receive from stream and write to stdout/stderr
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				errCh <- fmt.Errorf("receive from stream: %w", err)
				return
			}

			switch payload := resp.Payload.(type) {
			case *pb.ExecContainerResponse_ExecId:
				// This is sent first, we already processed it earlier
				// Just ignore duplicates
			case *pb.ExecContainerResponse_Stdout:
				if _, err := os.Stdout.Write(payload.Stdout); err != nil {
					errCh <- fmt.Errorf("write stdout: %w", err)
					return
				}
			case *pb.ExecContainerResponse_Stderr:
				if _, err := os.Stderr.Write(payload.Stderr); err != nil {
					errCh <- fmt.Errorf("write stderr: %w", err)
					return
				}
			case *pb.ExecContainerResponse_ExitCode:
				exitCode = int(payload.ExitCode)
				// Cancel context to stop stdin reader if it's still running
				cancel()
				// Return immediately after getting exit code to avoid waiting
				return
			case *pb.ExecContainerResponse_Error:
				errCh <- fmt.Errorf("exec error: %s", payload.Error)
				return
			}
		}
	}()

	// Wait for all goroutines to complete
	// The output goroutine will complete when it receives the exit code from the server
	// The stdin goroutine (if running) may be blocked on Read(), but cancel() signals it to stop
	wg.Wait()

	// Check if any goroutine reported an error
	select {
	case err := <-errCh:
		return exitCode, err
	default:
		return exitCode, nil
	}
}
