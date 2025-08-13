package api

import "strings"

// CaddySpec is the Caddy reverse proxy configuration for a service.
type CaddySpec struct {
	// Config contains the Caddy config (Caddyfile) content. It must not conflict with the Caddy configs
	// of other services.
	Config string
}

func (c *CaddySpec) Equals(other *CaddySpec) bool {
	if c == nil {
		return other == nil || strings.TrimSpace(other.Config) == ""
	}
	if other == nil {
		return strings.TrimSpace(c.Config) == ""
	}

	return strings.TrimSpace(c.Config) == strings.TrimSpace(other.Config)
}
