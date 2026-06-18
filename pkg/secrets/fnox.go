package secrets

import (
	"context"
	"os/exec"
	"strings"
)

// Fnox implements the use of the fnox secret manager.
//
// The pattern uc://fnox/profile/foo, will end up there as profile/foo. Where profile is optional, so
// foo is valid as well.
type Fnox struct{}

const fnox = "fnox"

func (f *Fnox) Secrets(ctx context.Context, pattern string) ([]Secret, error) {
	fields := strings.Split(pattern, "/")
	args := []string{}
	switch len(fields) {
	case 1:
		args = []string{"get", fields[0]}
	case 2:
		args = []string{"-P", fields[0], "get", fields[1]}
	default:
		return nil, ErrNotFound
	}

	cmd := exec.CommandContext(ctx, fnox, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, ErrAccessDenied
	}
	// only support a single secret
	return []Secret{
		{
			ID:       pattern,
			Value:    out,
			Provider: fnox,
		},
	}, nil
}
