package compose

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
)

const CaddyExtensionKey = "x-caddy"

type Caddy struct {
	Config string `yaml:"config" json:"config"`
}

// DecodeMapstructure decodes x-caddy extension from either a string or an object.
// When x-caddy is a string, it's mapped directly to the Config field.
func (c *Caddy) DecodeMapstructure(value any) error {
	switch v := value.(type) {
	case *Caddy:
		// Already decoded, happens when mapstructure is called after initial parsing.
		*c = *v
		return nil
	case string:
		// Handle x-caddy: "Caddyfile config"
		*c = Caddy{Config: v}
	case map[string]any:
		// Use mapstructure to decode the map directly to the struct.
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result:           c,
			ErrorUnused:      true,  // Error if there are extra keys not in the struct.
			WeaklyTypedInput: false, // Enforce strict type matching.
		})
		if err != nil {
			return fmt.Errorf("create decoder for x-caddy extension: %w", err)
		}
		if err := decoder.Decode(v); err != nil {
			return fmt.Errorf("decode x-caddy extension: %w", err)
		}
	default:
		return fmt.Errorf("invalid type %T for x-caddy extension: expected string or object", value)
	}
	return nil
}
