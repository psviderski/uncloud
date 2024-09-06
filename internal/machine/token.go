package machine

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
	"uncloud/internal/secret"
)

const (
	TokenPrefix = "mtkn:"
)

// Token represents the machine's token for joining a cluster.
type Token struct {
	PublicKey secret.Secret
	Endpoints []netip.AddrPort
}

// NewToken creates a new machine token with the given public key and endpoints.
func NewToken(publicKey secret.Secret, endpoints []netip.AddrPort) Token {
	return Token{
		PublicKey: publicKey,
		Endpoints: endpoints,
	}
}

// ParseToken decodes a machine token from the given string.
func ParseToken(s string) (Token, error) {
	if !strings.HasPrefix(s, TokenPrefix) {
		return Token{}, fmt.Errorf("invalid token prefix: %s", s)
	}
	decoded, err := base64.StdEncoding.DecodeString(s[len(TokenPrefix):])
	if err != nil {
		return Token{}, fmt.Errorf("decode token: %w", err)
	}
	var token Token
	if err = json.Unmarshal(decoded, &token); err != nil {
		return Token{}, fmt.Errorf("unmarshal token: %w", err)
	}
	return token, nil
}

// String returns the machine token encoded as a string.
func (t Token) String() (string, error) {
	js, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("marshal token: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(js)
	return TokenPrefix + encoded, nil
}
