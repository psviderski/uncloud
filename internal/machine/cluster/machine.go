package cluster

import (
	"fmt"
	"slices"
	"strings"

	"github.com/psviderski/uncloud/internal/secret"
)

// NewMachineID generates a new unique machine ID.
func NewMachineID() (string, error) {
	return secret.NewID()
}

// NewRandomMachineName generates a random machine name in the format "machine-xxxx".
func NewRandomMachineName() (string, error) {
	suffix, err := secret.RandomAlphaNumeric(4)
	if err != nil {
		return "", fmt.Errorf("generate random suffix: %w", err)
	}
	return "machine-" + suffix, nil
}

// DefaultMachineName derives a machine name from the given hostname, falling back to a random name if
// no valid name can be derived from it. It then ensures the name is unique among existing by appending
// a numeric suffix ("-1", "-2", etc.) if needed.
func DefaultMachineName(hostname string, existing []string) (string, error) {
	name := machineNameFromHostname(hostname)
	if name == "" {
		var err error
		if name, err = NewRandomMachineName(); err != nil {
			return "", err
		}
	}

	if !slices.Contains(existing, name) {
		return name, nil
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if !slices.Contains(existing, candidate) {
			return candidate, nil
		}
	}
}

// machineNameFromHostname derives a machine name from a hostname.
// It returns an empty string if no valid name can be derived.
func machineNameFromHostname(hostname string) string {
	// Use the first DNS label only, e.g. "web-1.example.com" becomes "web-1".
	label, _, _ := strings.Cut(hostname, ".")
	label = strings.ToLower(strings.TrimSpace(label))

	var b strings.Builder
	for _, r := range label {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	// Trim leading and trailing hyphens that may result from the sanitisation above.
	return strings.Trim(b.String(), "-")
}
