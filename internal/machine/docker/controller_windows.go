//go:build windows

package docker

import (
	"context"
	"fmt"
	"net/netip"
)

// EnsureUncloudNetwork is a stub for Windows.
func (c *Controller) EnsureUncloudNetwork(ctx context.Context, subnet netip.Prefix, dnsServer netip.Addr) error {
	return fmt.Errorf("not supported on Windows")
}

// Cleanup is a stub for Windows.
func (c *Controller) Cleanup() error {
	return fmt.Errorf("not supported on Windows")
}
