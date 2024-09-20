//go:build darwin

package machine

import (
	"context"
	"fmt"
)

// setupDockerNetwork is a stub for darwin.
func (nc *networkController) setupDockerNetwork(ctx context.Context) error {
	return fmt.Errorf("not supported on darwin")
}
