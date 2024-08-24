package secret

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type Secret []byte

// String returns the hex-encoded string representation of the secret.
//
//goland:noinspection GoMixedReceiverTypes
func (s Secret) String() string {
	return hex.EncodeToString(s)
}

//goland:noinspection GoMixedReceiverTypes
func (s Secret) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

//goland:noinspection GoMixedReceiverTypes
func (s *Secret) UnmarshalText(text []byte) error {
	decoded, err := hex.DecodeString(string(text))
	if err != nil {
		return fmt.Errorf("invalid hex-encoded secret: %w", err)
	}
	*s = decoded
	return nil
}

//goland:noinspection GoMixedReceiverTypes
func (s Secret) Equal(other Secret) bool {
	return bytes.Equal(s, other)
}

// New generates a random secret of the given length.
func New(len int) (Secret, error) {
	s := make(Secret, len)
	_, err := rand.Read(s)
	if err != nil {
		return Secret{}, err
	}
	return s, nil
}

func NewID() (string, error) {
	if s, err := New(16); err != nil {
		return "", err
	} else {
		return s.String(), nil
	}
}
