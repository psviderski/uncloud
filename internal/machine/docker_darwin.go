//go:build darwin

package machine

import (
	"context"
	"fmt"
	"net/netip"
)

// EnsureUncloudNetwork is a stub for darwin.
func (d *DockerManager) EnsureUncloudNetwork(ctx context.Context, subnet netip.Prefix) error {
	return fmt.Errorf("not supported on darwin")
}
