package compose

import (
	"context"

	"github.com/compose-spec/compose-go/v2/tree"
	"github.com/psviderski/uncloud/pkg/secrets"
)

func setSecrets(data any, _ tree.Path, _ bool) (any, error) {
	switch v := data.(type) {
	case map[string]any:
		// environment
		for name, pattern := range v {
			if x, ok := pattern.(string); ok {
				resolver, pattern, err := secrets.Parse(x)
				if err != nil {
					continue
				}
				secret, err := resolver.Secrets(context.TODO(), pattern)
				if err != nil {
					return data, err
				}
				v[name] = secret.Value
			}
		}
	}
	return data, nil
}
