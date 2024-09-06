package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"uncloud/internal/machine"
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
	return d.machine.Run(ctx)
}
