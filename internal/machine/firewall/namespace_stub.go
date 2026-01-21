//go:build !linux

package firewall

import (
	"context"
	"fmt"
	"net/netip"
)

const (
	IPSetPrefix                 = "uncloud-namespace-"
	UncloudNamespaceFilterChain = "UNCLOUD-NAMESPACE-FILTER"
)

func CreateNamespaceIPSet(_ context.Context, _ string) error {
	return fmt.Errorf("namespace isolation not supported on this platform")
}

func AddIPToNamespace(_ context.Context, _ netip.Addr, _ string) error {
	return fmt.Errorf("namespace isolation not supported on this platform")
}

func RemoveIPFromNamespace(_ context.Context, _ netip.Addr, _ string) error {
	return fmt.Errorf("namespace isolation not supported on this platform")
}

func ListNamespaces(_ context.Context) ([]string, error) {
	return nil, fmt.Errorf("namespace isolation not supported on this platform")
}

func UpdateNamespaceFilterRules(_ []string) error {
	return fmt.Errorf("namespace isolation not supported on this platform")
}
