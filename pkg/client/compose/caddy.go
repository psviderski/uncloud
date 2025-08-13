package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
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

// isCaddyfilePath determines if a string is likely a file path rather than inline Caddyfile config.
func isCaddyfilePath(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}

	// For simplicity, multi-line string is considered an inline Caddyfile content.
	return !strings.Contains(s, "\n")
}

// transformServicesCaddyExtension processes Caddy extensions to load configs from files if needed.
func transformServicesCaddyExtension(project *types.Project) (*types.Project, error) {
	return project.WithServicesTransform(func(name string, service types.ServiceConfig) (types.ServiceConfig, error) {
		ext, ok := service.Extensions[CaddyExtensionKey]
		if !ok {
			return service, nil
		}

		caddy, ok := ext.(Caddy)
		if !ok {
			return service, nil
		}

		// Load the Caddyfile config from file if it's a path and replace the path with its content.
		if isCaddyfilePath(caddy.Config) {
			configPath := caddy.Config
			if !filepath.IsAbs(configPath) {
				configPath = filepath.Join(project.WorkingDir, configPath)
			}

			content, err := os.ReadFile(configPath)
			if err != nil {
				return service, fmt.Errorf("read Caddy config (Caddyfile) from file '%s' for service '%s': %w",
					caddy.Config, name, err)
			}

			caddy.Config = string(content)
		}

		caddy.Config = strings.TrimSpace(caddy.Config)
		service.Extensions[CaddyExtensionKey] = caddy

		return service, nil
	})
}
