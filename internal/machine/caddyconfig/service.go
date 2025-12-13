package caddyconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Service provides methods to interact with the Caddy configuration on the machine
// and the running Caddy instance via its Admin API.
type Service struct {
	configDir string
	client    *CaddyAdminClient
}

// NewService creates a new Service instance with the specified Caddy configuration directory
// and Admin API socket path.
func NewService(configDir, adminSock string) *Service {
	return &Service{
		configDir: configDir,
		client:    NewCaddyAdminClient(adminSock),
	}
}

// Caddyfile retrieves the current Caddy configuration (Caddyfile) from the machine's config directory.
func (s *Service) Caddyfile() (string, time.Time, error) {
	path := filepath.Join(s.configDir, "Caddyfile")
	content, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("read Caddyfile from file '%s': %w", path, err)
	}

	// Get the file modification time.
	fileInfo, err := os.Stat(path)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("get Caddyfile file info '%s': %w", path, err)
	}

	return string(content), fileInfo.ModTime(), nil
}

// GetUpstreams retrieves the status of Caddy upstreams grouped by host.
func (s *Service) GetUpstreams(ctx context.Context) (map[string][]UpstreamStatus, error) {
	if !s.client.IsAvailable(ctx) {
		return nil, fmt.Errorf("caddy is not running")
	}

	// Get the flat list of upstream statuses.
	statuses, err := s.client.GetUpstreams(ctx)
	if err != nil {
		return nil, fmt.Errorf("get upstreams status: %w", err)
	}
	statusMap := make(map[string]UpstreamStatus, len(statuses))
	for _, st := range statuses {
		statusMap[st.Address] = st
	}

	// Get the current configuration to map upstreams to hosts.
	configJSON, err := s.client.GetConfigJSON(ctx)
	if err != nil {
		return nil, fmt.Errorf("get caddy config: %w", err)
	}

	var config caddyConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("parse caddy config: %w", err)
	}

	result := make(map[string][]UpstreamStatus)

	for _, server := range config.Apps.HTTP.Servers {
		collectUpstreams(server.Routes, nil, statusMap, result)
	}

	return result, nil
}

func collectUpstreams(routes []caddyRoute, parentHosts []string, statusMap map[string]UpstreamStatus, result map[string][]UpstreamStatus) {
	for _, route := range routes {
		// Calculate the hosts for this route.
		var currentHosts []string
		for _, m := range route.Match {
			currentHosts = append(currentHosts, m.Host...)
		}

		effectiveHosts := parentHosts
		if len(currentHosts) > 0 {
			if len(parentHosts) > 0 {
				// Intersection of hosts.
				var intersection []string
				for _, h1 := range parentHosts {
					for _, h2 := range currentHosts {
						if h1 == h2 {
							intersection = append(intersection, h1)
						}
					}
				}
				effectiveHosts = intersection
			} else {
				effectiveHosts = currentHosts
			}
		}

		for _, h := range route.Handle {
			if h.Handler == "reverse_proxy" {
				for _, u := range h.Upstreams {
					if st, ok := statusMap[u.Dial]; ok {
						for _, host := range effectiveHosts {
							result[host] = append(result[host], st)
						}
					}
				}
			} else if h.Handler == "subroute" {
				collectUpstreams(h.Routes, effectiveHosts, statusMap, result)
			}
		}
	}
}

type caddyConfig struct {
	Apps struct {
		HTTP struct {
			Servers map[string]caddyServer `json:"servers"`
		} `json:"http"`
	} `json:"apps"`
}

type caddyServer struct {
	Routes []caddyRoute `json:"routes"`
}

type caddyRoute struct {
	Match  []caddyMatch  `json:"match"`
	Handle []caddyHandle `json:"handle"`
}

type caddyMatch struct {
	Host []string `json:"host"`
}

type caddyHandle struct {
	Handler   string          `json:"handler"`
	Routes    []caddyRoute    `json:"routes"`    // For subroute
	Upstreams []caddyUpstream `json:"upstreams"` // For reverse_proxy
}

type caddyUpstream struct {
	Dial string `json:"dial"`
}
