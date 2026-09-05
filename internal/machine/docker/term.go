//go:build linux

package docker

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/moby/term"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"golang.org/x/sys/unix"
)

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
