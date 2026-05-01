package prometheus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const Port = 51004

type Server struct {
	*http.Server
	listenAddr netip.Addr
	log        *slog.Logger
}

func New(listenAddr netip.Addr) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{Handler: mux, ReadTimeout: 5 * time.Second}
	return &Server{server, listenAddr, slog.With("component", "prometheus")}
}

func (s *Server) Run(ctx context.Context) error {
	addr := net.JoinHostPort(s.listenAddr.String(), strconv.Itoa(Port))
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen prometheus server: %w", err)
	}

	errCh := make(chan error, 1)

	go func() {
		if err := s.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("prometheus server failed: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		s.stop()
		return err
	case <-ctx.Done():
		slog.Info("Stopping DNS server.")
		return s.stop()
	}
}

func (s *Server) stop() error {
	return s.Shutdown(context.TODO())
}
