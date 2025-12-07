//go:build !linux

package firewall

import (
	"fmt"
	"net/netip"
)

const (
	IPSetPrefix                  = "uncloud-namespace-"
	UncloudNamespaceFilterChain  = "UNCLOUD-NAMESPACE-FILTER"
)

func CreateNamespaceIPSet(namespace string) error               { return fmt.Errorf("namespace isolation not supported on this platform") }
func AddIPToNamespace(ip netip.Addr, namespace string) error    { return fmt.Errorf("namespace isolation not supported on this platform") }
func RemoveIPFromNamespace(ip netip.Addr, namespace string) error {
	return fmt.Errorf("namespace isolation not supported on this platform")
}
func ListNamespaces() ([]string, error) { return nil, fmt.Errorf("namespace isolation not supported on this platform") }
func FlushAllNamespaceIPSets() error    { return fmt.Errorf("namespace isolation not supported on this platform") }
func UpdateNamespaceFilterRules(namespaces []string) error {
	return fmt.Errorf("namespace isolation not supported on this platform")
}
