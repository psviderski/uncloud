package api

// CaddySpec is the Caddy reverse proxy configuration for a service.
type CaddySpec struct {
	// Config contains the Caddy config (Caddyfile) content. It must not conflict with the Caddy configs
	// of other services.
	Config string
}
