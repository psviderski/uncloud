package secret

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type Secret []byte

// FromHexString parses a hex-encoded string into a secret.
func FromHexString(s string) (Secret, error) {
	decoded, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid hex-encoded secret: %w", err)
	}
	return decoded, nil
}

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
	secret, err := FromHexString(string(text))
	if err != nil {
		return err
	}
	*s = secret
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
