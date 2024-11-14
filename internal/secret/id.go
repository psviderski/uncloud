package secret

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// NewID generates a unique 128-bit random ID as a hex-encoded string.
func NewID() (string, error) {
	if s, err := New(16); err != nil {
		return "", err
	} else {
		return s.String(), nil
	}
}

// RandomAlphaNumeric generates a random string of the specified length using the characters [a-z0-9].
func RandomAlphaNumeric(length int) (string, error) {
	const charSet = "abcdefghijklmnopqrstuvwxyz0123456789"
	s := make([]byte, length)
	for i := range s {
		randIdx, err := rand.Int(rand.Reader, big.NewInt(int64(len(charSet))))
		if err != nil {
			return "", fmt.Errorf("get random number: %w", err)
		}
		s[i] = charSet[randIdx.Int64()]
	}
	return string(s), nil
}
