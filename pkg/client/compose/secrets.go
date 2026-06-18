package compose

import (
	"fmt"

	"github.com/compose-spec/compose-go/v2/tree"
)

func setSecrets(data any, _ tree.Path, _ bool) (any, error) {
	switch v := data.(type) {
	case map[string]any:
		for name, secret := range v {
			fmt.Printf("%s %s\n", name, secret)
		}
	}
	return data, nil
}
