package caddyconfig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Service provides methods to interact with the Caddy configuration on the machine.
type Service struct {
	configDir string
	client    *CaddyAdminClient
}

// NewService creates a new Service instance with the specified Caddy configuration directory.
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

// GetUpstreams retrieves the status of Caddy upstreams.
func (s *Service) GetUpstreams(ctx context.Context) ([]UpstreamStatus, error) {
	if !s.client.IsAvailable(ctx) {
		return nil, fmt.Errorf("caddy is not running")
	}
	return s.client.GetUpstreams(ctx)
}
