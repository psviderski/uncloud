package firewall

import (
	"fmt"
	"net/netip"
	"os/exec"
	"strings"
)

const ipsetBinary = "ipset"

// IPSetPrefix is the prefix used for per-namespace ipsets.
const IPSetPrefix = "uncloud-namespace-"

// CreateNamespaceIPSet ensures an ipset exists for the given namespace.
func CreateNamespaceIPSet(namespace string) error {
	cmd := exec.Command(ipsetBinary, "create", IPSetPrefix+namespace, "hash:ip", "family", "inet", "-exist")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create ipset %s: %v: %s", namespace, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// AddIPToNamespace adds an IP to the namespace ipset.
func AddIPToNamespace(ip netip.Addr, namespace string) error {
	cmd := exec.Command(ipsetBinary, "add", IPSetPrefix+namespace, ip.String(), "-exist")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add ip %s to namespace %s: %v: %s", ip, namespace, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RemoveIPFromNamespace removes an IP from the namespace ipset.
func RemoveIPFromNamespace(ip netip.Addr, namespace string) error {
	cmd := exec.Command(ipsetBinary, "del", IPSetPrefix+namespace, ip.String(), "-exist")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("remove ip %s from namespace %s: %v: %s", ip, namespace, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ListNamespaces returns the namespaces for which ipsets exist.
func ListNamespaces() ([]string, error) {
	cmd := exec.Command(ipsetBinary, "list", "-name")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list ipsets: %v: %s", err, strings.TrimSpace(string(out)))
	}

	var namespaces []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, IPSetPrefix) {
			continue
		}
		ns := strings.TrimPrefix(line, IPSetPrefix)
		namespaces = append(namespaces, ns)
	}
	return namespaces, nil
}
