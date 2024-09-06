package cluster

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"uncloud/internal/secret"
)

// NewMachineID generates a new unique machine ID.
func NewMachineID() (string, error) {
	return secret.NewID()
}

// NewRandomMachineName generates a random machine name in the format "machine-xxxx".
func NewRandomMachineName() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	suffix := make([]byte, 4)
	for i := range suffix {
		randIdx, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("get random number: %w", err)
		}
		suffix[i] = charset[randIdx.Int64()]
	}
	return "machine-" + string(suffix), nil
}
