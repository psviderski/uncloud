package corrosion

import (
	"fmt"
	"log/slog"
	"os/exec"
)

const DefaultSystemdUnit = "uncloud-corrosion.service"

type SystemdService struct {
	DataDir string
	Unit    string
	User    string
}

func DefaultSystemdService(dataDir string) *SystemdService {
	return &SystemdService{
		DataDir: dataDir,
		Unit:    DefaultSystemdUnit,
		User:    DefaultUser,
	}
}

func (s *SystemdService) Configure() error {
	// TODO: create config.toml and schema dir in DataDir.
	return nil
}

func (s *SystemdService) Start() error {
	if _, err := exec.Command("systemctl", "start", s.Unit).Output(); err != nil {
		return fmt.Errorf("systemctl start %s: %w", s.Unit, err)
	}
	slog.Info("Corrosion systemd service started.", "unit", s.Unit)
	return nil
}
