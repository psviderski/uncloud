//go:build windows

package docker

import (
	"context"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

// handleTerminalResize is not supported on Windows.
func handleTerminalResize(ctx context.Context, inFd uintptr, stream pb.Docker_ExecContainerClient) error {
	return nil
}
