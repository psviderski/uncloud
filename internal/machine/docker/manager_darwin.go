//go:build darwin

package docker

import (
	"context"
	"fmt"
	"net/netip"
)

// EnsureUncloudNetwork is a stub for Darwin.
func (m *Manager) EnsureUncloudNetwork(ctx context.Context, subnet netip.Prefix, dnsServer netip.Addr) error {
	return fmt.Errorf("not supported on Darwin")
}
