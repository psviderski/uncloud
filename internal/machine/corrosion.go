package machine

import (
	"fmt"
	"log/slog"
	"os/exec"
)

const (
	CorrosionSystemdUnit = "uncloud-corrosion.service"
)

type CorrosionService interface {
	Configure() error
	Start() error
}

type CorrosionSystemdService struct {
	Unit    string
	DataDir string
}

func (s *CorrosionSystemdService) Configure() error {
	return nil
}

func (s *CorrosionSystemdService) Start() error {
	unit := s.Unit
	if unit == "" {
		unit = CorrosionSystemdUnit
	}

	if _, err := exec.Command("systemctl", "start", unit).Output(); err != nil {
		return fmt.Errorf("start %s: %w", unit, err)
	}
	slog.Info("Corrosion systemd service started.", "unit", unit)
	return nil
}
