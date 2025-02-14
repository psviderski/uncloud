//go:build darwin

package docker

import (
	"context"
	"fmt"
	"net/netip"
)

// EnsureUncloudNetwork is a stub for darwin.
func (m *Manager) EnsureUncloudNetwork(ctx context.Context, subnet netip.Prefix) error {
	return fmt.Errorf("not supported on darwin")
}
