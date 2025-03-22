package cluster

import (
	"fmt"
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
