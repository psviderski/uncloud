package proxy

import (
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"sync"
	"syscall"
	"time"
)

const (
	// TCPDialTimeout is the timeout for connecting to a backend.
	TCPDialTimeout = 10 * time.Second
)

// TCPProxy is a proxy for TCP connections with multi-backend load balancing.
// It follows patterns from docker/go-connections/proxy/tcp_proxy.go.
type TCPProxy struct {
	log          logger
	listener     *net.TCPListener
	frontendAddr *net.TCPAddr

	mu       sync.RWMutex
	backends []string // backend addresses (ip:port)
}

// NewTCPProxy creates a new TCPProxy listening on the given port.
func NewTCPProxy(port uint16, log *slog.Logger) (*TCPProxy, error) {
	addr := &net.TCPAddr{Port: int(port)}
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &TCPProxy{
		log:          loggerOrDefault(log),
		listener:     listener,
		frontendAddr: listener.Addr().(*net.TCPAddr),
	}, nil
}

// SetBackends updates the list of backend addresses.
func (p *TCPProxy) SetBackends(backends []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.backends = backends
}

// Backends returns a copy of the current backend list.
func (p *TCPProxy) Backends() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]string, len(p.backends))
	copy(result, p.backends)
	return result
}

// Run starts forwarding TCP connections. It blocks until Close is called.
func (p *TCPProxy) Run() {
	quit := make(chan struct{})
	defer close(quit)

	for {
		client, err := p.listener.Accept()
		if err != nil {
			p.log.Info("Stopping TCP proxy", "frontend", p.frontendAddr, "err", err)
			return
		}
		go p.clientLoop(client.(*net.TCPConn), quit)
	}
}

func (p *TCPProxy) clientLoop(client *net.TCPConn, quit chan struct{}) {
	defer client.Close()

	// Get shuffled copy of backends for retry
	backends := p.shuffledBackends()
	if len(backends) == 0 {
		p.log.Debug("No backends available", "frontend", p.frontendAddr, "client", client.RemoteAddr())
		return
	}

	// Try backends in shuffled order until one connects
	var backend *net.TCPConn
	for _, addr := range backends {
		conn, err := net.DialTimeout("tcp", addr, TCPDialTimeout)
		if err != nil {
			p.log.Warn("Backend dial failed, trying next", "backend", addr, "err", err)
			continue
		}
		backend = conn.(*net.TCPConn)
		break
	}

	if backend == nil {
		p.log.Error("All backends failed", "frontend", p.frontendAddr, "client", client.RemoteAddr())
		return
	}
	defer backend.Close()

	p.log.Debug("Proxying connection", "frontend", p.frontendAddr, "client", client.RemoteAddr(), "backend", backend.RemoteAddr())

	event := make(chan int64)
	broker := func(to, from *net.TCPConn) {
		written, err := io.Copy(to, from)
		if err != nil {
			// If the socket we are writing to is shutdown with
			// SHUT_WR, forward it to the other end of the pipe:
			if opErr, ok := err.(*net.OpError); ok && opErr.Err == syscall.EPIPE {
				_ = from.CloseRead()
			}
		}
		_ = to.CloseWrite()
		event <- written
	}

	go broker(client, backend)
	go broker(backend, client)

	for i := 0; i < 2; i++ {
		select {
		case <-event:
		case <-quit:
			// Interrupt the two brokers and "join" them.
			_ = client.Close()
			_ = backend.Close()
			for ; i < 2; i++ {
				<-event
			}
			return
		}
	}
}

func (p *TCPProxy) shuffledBackends() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.backends) == 0 {
		return nil
	}

	// Create a shuffled copy
	shuffled := make([]string, len(p.backends))
	copy(shuffled, p.backends)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	return shuffled
}

// Close stops forwarding traffic.
func (p *TCPProxy) Close() {
	_ = p.listener.Close()
}

// FrontendAddr returns the TCP address the proxy is listening on.
func (p *TCPProxy) FrontendAddr() net.Addr {
	return p.frontendAddr
}
