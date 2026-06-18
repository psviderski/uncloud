// Package secrets defines the interface to 3rd party secret managers. It resolves secrets to their actual
// data. This only reads secrets, settings and updating them is left out.
package secrets

import "context"

// Secret is a secret as resolved by a providing plugin.
type Secret struct {
	ID       string
	Value    []byte
	Provider string
}

// Providers is the list of plugins we have to resolve a secret.
var Providers = []string{"fnox"}

// Resolver is the interface all providers should implement.
type Resolver interface {
	Secrets(ctx context.Context, pattern string) ([]Secret, error)
}
