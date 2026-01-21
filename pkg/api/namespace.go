package api

import (
	"fmt"
	"regexp"
)

const (
	// DefaultNamespace is the default namespace for services when not explicitly specified.
	DefaultNamespace = "default"
	// SystemNamespace is the privileged namespace for infrastructure services (e.g., Caddy).
	// Services in this namespace can communicate with all other namespaces and are exempt from
	// namespace isolation rules.
	SystemNamespace = "uncloud-system"
)

// NamespaceNameRegexp validates namespace names: lowercase alphanumeric with hyphens, starting and ending with alphanumeric.
// Max 63 chars (DNS label limit).
var NamespaceNameRegexp = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ValidateNamespaceName validates a namespace name according to DNS label rules.
// An empty namespace is considered invalid; use ValidateOptionalNamespace for optional values.
func ValidateNamespaceName(name string) error {
	if name == "" {
		return fmt.Errorf("namespace cannot be empty")
	}
	if len(name) > 63 {
		return fmt.Errorf("namespace too long: max 63 chars")
	}
	if !NamespaceNameRegexp.MatchString(name) {
		return fmt.Errorf("invalid namespace %q: must be lowercase alphanumeric with hyphens, starting and ending with alphanumeric", name)
	}
	return nil
}

// ValidateOptionalNamespace validates a namespace name if provided, allowing empty values.
// This is useful for CLI flags where an empty namespace means "no filter" or "use default".
func ValidateOptionalNamespace(name string) error {
	if name == "" {
		return nil
	}
	return ValidateNamespaceName(name)
}
