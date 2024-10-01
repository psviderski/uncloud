package corroservice

import (
	"fmt"
	"log/slog"
	"os/exec"
)

const DefaultSystemdUnit = "uncloud-corrosion.service"

type SystemdService struct {
	DataDir string
	Unit    string
	running bool
}

func DefaultSystemdService(dataDir string) *SystemdService {
	return &SystemdService{
		DataDir: dataDir,
		Unit:    DefaultSystemdUnit,
	}
}

func (s *SystemdService) Start() error {
	if _, err := exec.Command("systemctl", "start", s.Unit).Output(); err != nil {
		return fmt.Errorf("systemctl start %s: %w", s.Unit, err)
	}
	slog.Info("Corrosion systemd service started.", "unit", s.Unit)

	// TODO: run a goroutine to check the status of the service and log any errors in the uncloud log.
	s.running = true
	return nil
}

func (s *SystemdService) Restart() error {
	if _, err := exec.Command("systemctl", "restart", s.Unit).Output(); err != nil {
		return fmt.Errorf("systemctl restart %s: %w", s.Unit, err)
	}
	slog.Info("Corrosion systemd service restarted.", "unit", s.Unit)

	// TODO: run a goroutine to check the status of the service and log any errors in the uncloud log.
	s.running = true
	return nil
}

func (s *SystemdService) Running() bool {
	return s.running
}
