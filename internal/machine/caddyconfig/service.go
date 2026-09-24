package caddyconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Service provides methods to interact with the Caddy configuration on the machine.
type Service struct {
	configDir               string
	mu                      sync.RWMutex
	lastReconciliationError error
}

// NewService creates a new Service instance with the specified Caddy configuration directory.
func NewService(configDir string) *Service {
	return &Service{configDir: configDir}
}

// Caddyfile retrieves the saved Caddyfile from the machine's config directory. The saved file may differ from the
// running configuration if Caddy accepted a load but the subsequent write failed.
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

// LastReconciliationError reports the most recent unsuccessful controller attempt, if any. A successful attempt
// clears it. An empty value does not prove that Caddy has loaded the saved Caddyfile.
func (s *Service) LastReconciliationError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastReconciliationError
}

func (s *Service) setReconciliationResult(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastReconciliationError = err
}
