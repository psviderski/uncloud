package api

import (
	"fmt"
	"regexp"
)

const (
	// DefaultNamespace is the default namespace for services when not explicitly specified.
	DefaultNamespace = "default"
)

var (
	// NamespaceNameRegexp validates namespace names: lowercase alphanumeric with hyphens, starting and ending with alphanumeric.
	// Max 63 chars (DNS label limit).
	NamespaceNameRegexp = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
)

// ValidateNamespaceName validates a namespace name according to DNS label rules.
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
