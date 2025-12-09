package tcpproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"sync"
	"time"
)

var (
	ErrNoBackends  = errors.New("no backends available")
	ErrPortInUse   = errors.New("port already in use")
	ErrProxyClosed = errors.New("proxy is closed")
)

// Proxy is a multi-port TCP proxy with port-based routing.
// Each published port maps to a set of backend container addresses.
type Proxy struct {
	mu        sync.RWMutex
	listeners map[uint16]*portListener // published_port → listener
	backends  map[uint16][]string      // published_port → container addrs (ip:port)

	log    *slog.Logger
	closed bool
}

// portListener manages a listener on a specific port.
type portListener struct {
	port     uint16
	listener net.Listener
	cancel   context.CancelFunc
}

// NewProxy creates a new TCP proxy instance.
func NewProxy(log *slog.Logger) *Proxy {
	if log == nil {
		log = slog.Default()
	}
	return &Proxy{
		listeners: make(map[uint16]*portListener),
		backends:  make(map[uint16][]string),
		log:       log.With("component", "tcp-proxy"),
	}
}

// SetBackends sets the backend addresses for a published port.
// If the port doesn't have a listener yet, one will be started.
// If backends is empty, the listener for that port will be stopped.
func (p *Proxy) SetBackends(ctx context.Context, publishedPort uint16, backends []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrProxyClosed
	}

	// If no backends, remove the port entirely
	if len(backends) == 0 {
		return p.removePortLocked(publishedPort)
	}

	// Update backends
	p.backends[publishedPort] = backends

	// Ensure listener exists for this port
	if _, exists := p.listeners[publishedPort]; !exists {
		if err := p.startListenerLocked(ctx, publishedPort); err != nil {
			delete(p.backends, publishedPort)
			return err
		}
	}

	p.log.Info("Backends updated", "port", publishedPort, "backends", len(backends))
	return nil
}

// RemovePort stops the listener and removes backends for a port.
func (p *Proxy) RemovePort(publishedPort uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrProxyClosed
	}

	return p.removePortLocked(publishedPort)
}

func (p *Proxy) removePortLocked(port uint16) error {
	delete(p.backends, port)

	if pl, ok := p.listeners[port]; ok {
		pl.cancel()
		pl.listener.Close()
		delete(p.listeners, port)
		p.log.Info("Stopped listener", "port", port)
	}

	return nil
}

// startListenerLocked starts a listener on the given port. Must be called with mu held.
func (p *Proxy) startListenerLocked(ctx context.Context, port uint16) error {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPortInUse, err)
	}

	listenerCtx, cancel := context.WithCancel(ctx)
	pl := &portListener{
		port:     port,
		listener: listener,
		cancel:   cancel,
	}
	p.listeners[port] = pl

	go p.acceptLoop(listenerCtx, pl)

	p.log.Info("Started listener", "port", port)
	return nil
}

// acceptLoop accepts connections on a listener and handles them.
func (p *Proxy) acceptLoop(ctx context.Context, pl *portListener) {
	for {
		conn, err := pl.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				p.log.Error("Accept failed", "port", pl.port, "err", err)
				continue
			}
		}

		go p.handleConnection(conn, pl.port)
	}
}

// handleConnection handles a single TCP connection.
func (p *Proxy) handleConnection(conn net.Conn, port uint16) {
	defer conn.Close()

	// Get backends for this port
	p.mu.RLock()
	backends := p.backends[port]
	p.mu.RUnlock()

	if len(backends) == 0 {
		p.log.Debug("No backends for port", "port", port, "remote", conn.RemoteAddr())
		return
	}

	// Random load balancing
	backend := backends[rand.IntN(len(backends))]

	// Connect to backend
	upstream, err := net.DialTimeout("tcp", backend, 10*time.Second)
	if err != nil {
		p.log.Error("Failed to connect to backend", "port", port, "backend", backend, "err", err)
		return
	}
	defer upstream.Close()

	p.log.Debug("Proxying connection", "port", port, "remote", conn.RemoteAddr(), "backend", backend)

	// Bidirectional copy
	var wg sync.WaitGroup
	wg.Add(2)

	// Client → Backend
	go func() {
		defer wg.Done()
		io.Copy(upstream, conn)
		if tc, ok := upstream.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	// Backend → Client
	go func() {
		defer wg.Done()
		io.Copy(conn, upstream)
		if tc, ok := conn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	wg.Wait()
}

// Close stops all listeners and cleans up.
func (p *Proxy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true

	for port, pl := range p.listeners {
		pl.cancel()
		pl.listener.Close()
		delete(p.listeners, port)
	}

	p.backends = make(map[uint16][]string)

	p.log.Info("TCP proxy closed")
	return nil
}

// ListeningPorts returns a list of ports the proxy is currently listening on.
func (p *Proxy) ListeningPorts() []uint16 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ports := make([]uint16, 0, len(p.listeners))
	for port := range p.listeners {
		ports = append(ports, port)
	}
	return ports
}

// PortCount returns the number of active ports.
func (p *Proxy) PortCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.listeners)
}
