package corroservice

import (
	"context"
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

func (s *SystemdService) Start(ctx context.Context) error {
	return s.startOrRestart(ctx, "start")
}

func (s *SystemdService) Stop(_ context.Context) error {
	if _, err := exec.Command("systemctl", "stop", s.Unit).Output(); err != nil {
		return fmt.Errorf("systemctl stop %s: %w", s.Unit, err)
	}
	slog.Info("Corrosion systemd service stopped.", "unit", s.Unit)

	return nil
}

func (s *SystemdService) Restart(ctx context.Context) error {
	return s.startOrRestart(ctx, "restart")
}

func (s *SystemdService) startOrRestart(ctx context.Context, cmd string) error {
	if _, err := exec.Command("systemctl", cmd, s.Unit).Output(); err != nil {
		return fmt.Errorf("systemctl %s %s: %w", cmd, s.Unit, err)
	}
	slog.Debug(fmt.Sprintf("Corrosion systemd service %sed.", cmd), "unit", s.Unit)

	slog.Debug("Waiting for corrosion service to be ready.")
	if err := WaitReady(ctx, s.DataDir); err != nil {
		return err
	}
	slog.Debug("Corrosion service is ready.")
	s.running = true

	return nil
}

func (s *SystemdService) Running() bool {
	return s.running
}
