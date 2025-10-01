package firewall

import (
	"fmt"
	"net/netip"
)

// ConfigureIptablesChains is a stub for Darwin.
func ConfigureIptablesChains(machineIP netip.Addr) error {
	return fmt.Errorf("not supported on Darwin")
}

// CleanupIptablesChains is a stub for Darwin.
func CleanupIptablesChains() error {
	return fmt.Errorf("not supported on Darwin")
}
