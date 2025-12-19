package proxy

import (
	"encoding/binary"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net"
	"sync"
	"syscall"
	"time"
)

const (
	// UDPConnTrackTimeout is the timeout used for UDP connection tracking.
	UDPConnTrackTimeout = 90 * time.Second
	// UDPBufSize is the buffer size for UDP packets.
	UDPBufSize = 65507
)

// connTrackKey is a net.Addr where the IP is split into two fields so it can
// be used as a map key.
type connTrackKey struct {
	IPHigh uint64
	IPLow  uint64
	Port   int
}

func newConnTrackKey(addr *net.UDPAddr) *connTrackKey {
	if len(addr.IP) == net.IPv4len {
		return &connTrackKey{
			IPHigh: 0,
			IPLow:  uint64(binary.BigEndian.Uint32(addr.IP)),
			Port:   addr.Port,
		}
	}
	return &connTrackKey{
		IPHigh: binary.BigEndian.Uint64(addr.IP[:8]),
		IPLow:  binary.BigEndian.Uint64(addr.IP[8:]),
		Port:   addr.Port,
	}
}

type connTrackMap map[connTrackKey]*net.UDPConn

// UDPProxy is a proxy for UDP datagrams with multi-backend load balancing.
// It follows patterns from docker/go-connections/proxy/udp_proxy.go.
type UDPProxy struct {
	log          logger
	listener     *net.UDPConn
	frontendAddr *net.UDPAddr

	mu       sync.RWMutex
	backends []string // backend addresses (ip:port)

	connTrackTable connTrackMap
	connTrackLock  sync.Mutex
}

// NewUDPProxy creates a new UDPProxy listening on the given port.
func NewUDPProxy(port uint16, log *slog.Logger) (*UDPProxy, error) {
	addr := &net.UDPAddr{Port: int(port)}
	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	return &UDPProxy{
		log:            loggerOrDefault(log),
		listener:       listener,
		frontendAddr:   listener.LocalAddr().(*net.UDPAddr),
		connTrackTable: make(connTrackMap),
	}, nil
}

// SetBackends updates the list of backend addresses.
func (p *UDPProxy) SetBackends(backends []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.backends = backends
}

// Backends returns a copy of the current backend list.
func (p *UDPProxy) Backends() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]string, len(p.backends))
	copy(result, p.backends)
	return result
}

func (p *UDPProxy) replyLoop(proxyConn *net.UDPConn, clientAddr *net.UDPAddr, clientKey *connTrackKey) {
	defer func() {
		p.connTrackLock.Lock()
		delete(p.connTrackTable, *clientKey)
		p.connTrackLock.Unlock()
		_ = proxyConn.Close()
	}()

	readBuf := make([]byte, UDPBufSize)
	for {
		_ = proxyConn.SetReadDeadline(time.Now().Add(UDPConnTrackTimeout))
	again:
		read, err := proxyConn.Read(readBuf)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Err == syscall.ECONNREFUSED {
				// This will happen if the last write failed
				// (e.g: nothing is actually listening on the
				// proxied port on the container), ignore it
				// and continue until UDPConnTrackTimeout expires.
				goto again
			}
			return
		}
		for i := 0; i != read; {
			written, err := p.listener.WriteToUDP(readBuf[i:read], clientAddr)
			if err != nil {
				return
			}
			i += written
		}
	}
}

// Run starts forwarding UDP datagrams. It blocks until Close is called.
func (p *UDPProxy) Run() {
	readBuf := make([]byte, UDPBufSize)
	for {
		read, from, err := p.listener.ReadFromUDP(readBuf)
		if err != nil {
			// NOTE: Apparently ReadFrom doesn't return ECONNREFUSED like
			// Read does (see comment in replyLoop).
			if !errors.Is(err, net.ErrClosed) {
				p.log.Info("Stopping UDP proxy", "frontend", p.frontendAddr, "err", err)
			}
			break
		}

		fromKey := newConnTrackKey(from)
		p.connTrackLock.Lock()
		proxyConn, hit := p.connTrackTable[*fromKey]
		if !hit {
			// Pick a random backend for this new client session
			backend := p.randomBackend()
			if backend == "" {
				p.log.Debug("No backends available", "frontend", p.frontendAddr, "client", from)
				p.connTrackLock.Unlock()
				continue
			}

			backendAddr, err := net.ResolveUDPAddr("udp", backend)
			if err != nil {
				p.log.Warn("Failed to resolve backend", "backend", backend, "err", err)
				p.connTrackLock.Unlock()
				continue
			}

			proxyConn, err = net.DialUDP("udp", nil, backendAddr)
			if err != nil {
				p.log.Warn("Failed to dial backend", "backend", backend, "err", err)
				p.connTrackLock.Unlock()
				continue
			}
			p.connTrackTable[*fromKey] = proxyConn
			go p.replyLoop(proxyConn, from, fromKey)

			p.log.Debug("New UDP session", "frontend", p.frontendAddr, "client", from, "backend", backend)
		}
		p.connTrackLock.Unlock()

		for i := 0; i != read; {
			written, err := proxyConn.Write(readBuf[i:read])
			if err != nil {
				p.log.Warn("Failed to write to backend", "err", err)
				break
			}
			i += written
		}
	}
}

func (p *UDPProxy) randomBackend() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.backends) == 0 {
		return ""
	}
	return p.backends[rand.IntN(len(p.backends))]
}

// Close stops forwarding traffic.
func (p *UDPProxy) Close() {
	_ = p.listener.Close()
	p.connTrackLock.Lock()
	defer p.connTrackLock.Unlock()
	for _, conn := range p.connTrackTable {
		_ = conn.Close()
	}
}

// FrontendAddr returns the UDP address the proxy is listening on.
func (p *UDPProxy) FrontendAddr() net.Addr {
	return p.frontendAddr
}
