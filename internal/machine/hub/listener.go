package hub

import (
	"context"
	"net"
	"sync"

	"github.com/psviderski/uncloud/hub/tunnel"
)

// connListener is a net.Listener that accepts the tunnel connections pushed to it. It lets one gRPC server serve
// a new tunnel after each reconnect.
type connListener struct {
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

var _ net.Listener = (*connListener)(nil)

func newConnListener() *connListener {
	return &connListener{
		conns:  make(chan net.Conn),
		closed: make(chan struct{}),
	}
}

// push hands the connection to Accept. It returns an error if the listener is closed or ctx is cancelled first.
func (l *connListener) push(ctx context.Context, conn net.Conn) error {
	select {
	case l.conns <- conn:
		return nil
	case <-l.closed:
		return net.ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Accept implements net.Listener.
func (l *connListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

// Close implements net.Listener.
func (l *connListener) Close() error {
	l.once.Do(func() {
		close(l.closed)
	})
	return nil
}

// Addr implements net.Listener.
func (l *connListener) Addr() net.Addr {
	return tunnel.Addr("hub-tunnels")
}
