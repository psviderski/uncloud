package daemon

import (
	"context"
	"fmt"
	systemd "github.com/coreos/go-systemd/daemon"
	"log/slog"
	"github.com/psviderski/uncloud/internal/machine"
)

type Daemon struct {
	machine *machine.Machine
}

func New(dataDir string) (*Daemon, error) {
	config := &machine.Config{
		DataDir: dataDir,
	}
	mach, err := machine.NewMachine(config)
	if err != nil {
		return nil, fmt.Errorf("init machine: %w", err)
	}

	return &Daemon{
		machine: mach,
	}, nil
}

func (d *Daemon) Run(ctx context.Context) error {
	slog.Info("Starting machine.")

	// Notify systemd that the daemon is ready when the machine is started.
	go func() {
		select {
		case <-d.machine.Started():
			_, err := systemd.SdNotify(false, systemd.SdNotifyReady)
			if err != nil {
				slog.Error("Failed to notify systemd that the daemon is ready.", "err", err)
			}
		case <-ctx.Done():
		}
	}()

	return d.machine.Run(ctx)
}
