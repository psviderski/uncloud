package corroservice

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"time"
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

func (s *SystemdService) Restart(ctx context.Context) error {
	return s.startOrRestart(ctx, "restart")
}

func (s *SystemdService) startOrRestart(ctx context.Context, cmd string) error {
	if _, err := exec.Command("systemctl", cmd, s.Unit).Output(); err != nil {
		return fmt.Errorf("systemctl %s %s: %w", cmd, s.Unit, err)
	}
	slog.Info(fmt.Sprintf("Corrosion systemd service %sed.", cmd), "unit", s.Unit)

	// Optimistically wait for the corrosion service to start and initialise the database schema before proceeding.
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-ctx.Done():
		return nil
	}

	// TODO: run a goroutine to check the status of the service and log any errors in the uncloud log.
	s.running = true
	return nil
}

func (s *SystemdService) Running() bool {
	return s.running
}
