// Package secrets defines the interface to 3rd party secret managers. It resolves secrets to their actual
// data. This only reads secrets, settings and updating them is left out.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
)

const Scheme = "uc"

// Secret is a secret as resolved by a providing plugin.
type Secret struct {
	ID       string
	Value    []byte
	Provider string
}

var (
	ErrNotFound     = errors.New("secret not found")
	ErrAccessDenied = errors.New("access denied")
)

// Providers is the list of plugins we have to resolve a secret.
var Providers = map[string]Resolver{
	fnox: &Fnox{},
}

// Resolver is the interface all providers should implement.
type Resolver interface {
	Secrets(ctx context.Context, pattern string) ([]Secret, error)
}

// Parse parses a pattern uc://<provider>/bla/foo and returns the provider
// and the secrets pattern that is used. The provider implementation knows how to deal with the pattern.
func Parse(pointer string) (provider, pattern string, err error) {
	u, err := url.Parse(pointer)
	if err != nil {
		return "", "", err
	}
	if u.Scheme != Scheme {
		return "", "", fmt.Errorf("unknown scheme: %s", u.Scheme)
	}
	// first path element is provider, rest is the key's pattern.
	provider, pattern = path.Split(u.Path)
	return provider, pattern, nil
}
