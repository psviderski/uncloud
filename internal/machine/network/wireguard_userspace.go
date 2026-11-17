//go:build linux

package network

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/ipc"
	"golang.zx2c4.com/wireguard/tun"
)

type userspaceWireGuard struct {
	device    *device.Device
	listener  net.Listener
	closing   chan struct{}
	done      chan error
	closeOnce sync.Once
}

func startUserspaceWireGuard(name string) (*userspaceWireGuard, error) {
	tunDevice, err := tun.CreateTUN(name, device.DefaultMTU)
	if err != nil {
		return nil, fmt.Errorf("create TUN device: %w", err)
	}

	interfaceName, err := tunDevice.Name()
	if err != nil {
		_ = tunDevice.Close()
		return nil, fmt.Errorf("get TUN device name: %w", err)
	}

	uapiFile, err := ipc.UAPIOpen(interfaceName)
	if err != nil {
		_ = tunDevice.Close()
		return nil, fmt.Errorf("open UAPI socket: %w", err)
	}

	logger := &device.Logger{
		Verbosef: func(format string, args ...any) {
			slog.Debug(fmt.Sprintf(format, args...), "name", interfaceName)
		},
		Errorf: func(format string, args ...any) {
			slog.Error(fmt.Sprintf(format, args...), "name", interfaceName)
		},
	}
	wgDevice := device.NewDevice(tunDevice, conn.NewDefaultBind(), logger)
	listener, err := ipc.UAPIListen(interfaceName, uapiFile)
	_ = uapiFile.Close()
	if err != nil {
		wgDevice.Close()
		return nil, fmt.Errorf("listen on UAPI socket: %w", err)
	}

	userspace := &userspaceWireGuard{
		device:   wgDevice,
		listener: listener,
		closing:  make(chan struct{}),
		done:     make(chan error, 1),
	}
	go userspace.serveUAPI(interfaceName)
	go userspace.watchDevice(interfaceName)
	return userspace, nil
}

func (w *userspaceWireGuard) serveUAPI(name string) {
	for {
		conn, err := w.listener.Accept()
		if err != nil {
			select {
			case <-w.closing:
				return
			default:
				slog.Error("Userspace WireGuard UAPI listener stopped.", "name", name, "err", err)
				w.reportError(fmt.Errorf("userspace WireGuard UAPI listener stopped: %w", err))
				w.device.Close()
				return
			}
		}
		go w.device.IpcHandle(conn)
	}
}

func (w *userspaceWireGuard) watchDevice(name string) {
	<-w.device.Wait()
	select {
	case <-w.closing:
		return
	default:
	}

	w.reportError(errors.New("userspace WireGuard device stopped"))
	if err := w.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		slog.Warn("Failed to close stopped userspace WireGuard UAPI listener.", "name", name, "err", err)
	}
}

func (w *userspaceWireGuard) reportError(err error) {
	select {
	case w.done <- err:
	default:
	}
}

func (w *userspaceWireGuard) Close() {
	w.closeOnce.Do(func() {
		close(w.closing)
		if err := w.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			slog.Warn("Failed to close userspace WireGuard UAPI listener.", "err", err)
		}
		w.device.Close()
	})
}
