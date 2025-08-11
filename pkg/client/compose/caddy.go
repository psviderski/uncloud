package compose

import "fmt"

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
		// Handle x-caddy: "caddyfile config"
		*c = Caddy{Config: v}
	case map[string]any:
		// Handle the long syntax with a config key.
		if config, ok := v["config"]; ok {
			configStr, ok := config.(string)
			if !ok {
				return fmt.Errorf("x-caddy.config must be a string, got %T", config)
			}
			*c = Caddy{Config: configStr}
		}
	default:
		return fmt.Errorf("invalid type %T for x-caddy extension: expected string or object", value)
	}
	return nil
}
