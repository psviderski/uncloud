package token

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"github.com/psviderski/uncloud/internal/secret"
)

const (
	Prefix = "mtkn:"
)

// Token represents the machine's token for joining a cluster.
type Token struct {
	PublicKey secret.Secret
	PublicIP  netip.Addr
	Endpoints []netip.AddrPort
}

// New creates a new machine token with the given public key and endpoints.
func New(publicKey secret.Secret, publicIP netip.Addr, endpoints []netip.AddrPort) Token {
	return Token{
		PublicKey: publicKey,
		PublicIP:  publicIP,
		Endpoints: endpoints,
	}
}

// Parse decodes a machine token from the given string.
func Parse(s string) (Token, error) {
	if !strings.HasPrefix(s, Prefix) {
		return Token{}, fmt.Errorf("invalid token prefix: %s", s)
	}
	decoded, err := base64.StdEncoding.DecodeString(s[len(Prefix):])
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
	return Prefix + encoded, nil
}
