package metrics

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

const Port = 51090

type Server struct {
	*http.Server
	listenAddr netip.Addr
	log        *slog.Logger
}

func New(listenAddr netip.Addr) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{Handler: mux, ReadTimeout: 5 * time.Second}
	return &Server{server, listenAddr, slog.With("component", "metrics")}
}

func (s *Server) Run(ctx context.Context) error {
	addr := net.JoinHostPort(s.listenAddr.String(), strconv.Itoa(Port))
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen metrics server: %w", err)
	}

	errCh := make(chan error, 1)

	go func() {
		if err := s.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("metrics server failed: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		// The server failed on its own so there are no connections to drain.
		return err
	case <-ctx.Done():
		s.log.Info("Stopping metrics server.")
		// ctx is already cancelled so a graceful drain needs a new one with a reasonable timeout.
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.Shutdown(stopCtx)
	}
}
