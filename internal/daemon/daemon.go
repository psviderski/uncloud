package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/coreos/go-systemd/activation"
	systemd "github.com/coreos/go-systemd/daemon"
	"github.com/psviderski/uncloud/internal/machine"
)

const systemdSocketUnit = "uncloud.socket"

type Daemon struct {
	machine *machine.Machine
}

func New(dataDir string) (*Daemon, error) {
	listeners, err := activation.ListenersWithNames()
	if err != nil {
		return nil, fmt.Errorf("get systemd-activated sockets: %w", err)
	}

	listener, err := selectActivatedListener(listeners)
	if err != nil {
		return nil, err
	}
	if listener != nil {
		slog.Info("Using systemd-activated API socket.", "addr", listener.Addr().String())
	}

	config := &machine.Config{
		DataDir:            dataDir,
		ClusterAPISockPath: machine.DefaultClusterAPISockPath,
		ClusterAPIListener: listener,
	}
	mach, err := machine.NewMachine(config)
	if err != nil {
		initErr := fmt.Errorf("init machine: %w", err)
		if listener != nil {
			if closeErr := listener.Close(); closeErr != nil {
				initErr = errors.Join(initErr, fmt.Errorf("close systemd-activated socket: %w", closeErr))
			}
		}
		return nil, initErr
	}

	return &Daemon{
		machine: mach,
	}, nil
}

// selectActivatedListener returns the systemd activated listener. If systemd supplied multiple listeners,
// it returns the one associated with systemdSocketUnit.
func selectActivatedListener(listeners map[string][]net.Listener) (net.Listener, error) {
	var activated []net.Listener
	for _, named := range listeners {
		activated = append(activated, named...)
	}

	if len(activated) == 0 {
		return nil, nil
	}
	if len(activated) == 1 {
		return activated[0], nil
	}

	named, ok := listeners[systemdSocketUnit]
	if !ok || len(named) != 1 {
		return nil, fmt.Errorf("expected one systemd-activated socket named '%s', received %d",
			systemdSocketUnit, len(named))
	}

	// Close all other unused listeners.
	for name, unused := range listeners {
		if name == systemdSocketUnit {
			continue
		}
		for _, l := range unused {
			_ = l.Close()
		}
	}

	return named[0], nil
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
