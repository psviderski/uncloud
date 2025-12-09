package compose

import (
	"fmt"
	"strings"
)

const NamespaceExtensionKey = "x-namespace"

// NamespaceSource represents the source value for the namespace extension.
type NamespaceSource string

// DecodeMapstructure allows compose-go to decode the x-namespace extension from YAML.
func (s *NamespaceSource) DecodeMapstructure(value any) error {
	switch v := value.(type) {
	case NamespaceSource:
		*s = v
		return nil
	case *NamespaceSource:
		if v == nil {
			*s = ""
			return nil
		}
		*s = *v
		return nil
	case string:
		*s = NamespaceSource(strings.TrimSpace(v))
		return nil
	default:
		return fmt.Errorf("invalid type %T for x-namespace extension: expected string", value)
	}
}
