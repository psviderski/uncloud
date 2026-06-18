package compose

import (
	"fmt"

	"github.com/compose-spec/compose-go/v2/tree"
)

func secretsEngine(data any, _ tree.Path, _ bool) (any, error) {
	fmt.Printf("HALLO %v\n", data)
	return data, nil
}
