package firewall

import (
	"fmt"
	"net/netip"
)

// ConfigureIptablesChains is a stub for Windows.
func ConfigureIptablesChains(machineIP netip.Addr, wgPort int) error {
	return fmt.Errorf("not supported on Windows")
}

// CleanupIptablesChains is a stub for Windows.
func CleanupIptablesChains() error {
	return fmt.Errorf("not supported on Windows")
}
