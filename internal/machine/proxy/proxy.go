// Package proxy provides TCP and UDP proxying with multi-backend load balancing.
// The implementation follows patterns from docker/go-connections/proxy.
package proxy

import (
	"log/slog"
	"net"
)

// Proxy defines the behavior of a network proxy.
type Proxy interface {
	// Run starts the proxy and blocks until Close is called.
	Run()
	// Close stops the proxy.
	Close()
	// FrontendAddr returns the address the proxy is listening on.
	FrontendAddr() net.Addr
}

type logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// noopLogger is a logger that does nothing.
type noopLogger struct{}

func (l *noopLogger) Debug(msg string, args ...any) {}
func (l *noopLogger) Info(msg string, args ...any)  {}
func (l *noopLogger) Warn(msg string, args ...any)  {}
func (l *noopLogger) Error(msg string, args ...any) {}

func loggerOrDefault(log *slog.Logger) logger {
	if log == nil {
		return &noopLogger{}
	}
	return log
}
