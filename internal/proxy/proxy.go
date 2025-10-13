package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

// Proxy proxies local connections to a remote TCP address optionally using a custom dialer.
type Proxy struct {
	Listener    net.Listener
	RemoteAddr  string
	DialContext func(ctx context.Context, network, address string) (net.Conn, error)
	OnError     func(error)
	activeConns sync.WaitGroup
}

// deadliner is an interface for listeners that support setting deadlines.
type deadliner interface {
	SetDeadline(t time.Time) error
}

// halfCloser is an interface for connections that support half-close.
type halfCloser interface {
	CloseWrite() error
}

// Run starts the proxy and runs until the context is canceled.
func (p *Proxy) Run(ctx context.Context) {
	if p.DialContext == nil {
		p.DialContext = (&net.Dialer{}).DialContext
	}

	defer p.Listener.Close()

	// Handle incoming connections until context is canceled.
Loop:
	for {
		select {
		case <-ctx.Done():
			break Loop
		default:
		}

		// Set a deadline on the listener if supported to check context periodically.
		if dl, ok := p.Listener.(deadliner); ok {
			dl.SetDeadline(time.Now().Add(1 * time.Second))
		}

		conn, err := p.Listener.Accept()
		if err != nil {
			if os.IsTimeout(err) {
				// Just a timeout, continue to check context and accept again.
				continue
			}

			select {
			case <-ctx.Done():
				break Loop
			default:
				if p.OnError != nil {
					p.OnError(fmt.Errorf("accept local connection: %w", err))
				}
				continue
			}
		}

		p.activeConns.Add(1)
		go p.handleConnection(ctx, conn)
	}

	// Wait for all connections to finish.
	p.activeConns.Wait()
}

func (p *Proxy) handleConnection(ctx context.Context, localConn net.Conn) {
	defer p.activeConns.Done()
	defer localConn.Close()

	// Use a separate context with timeout for dialing the remote address.
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	remoteConn, err := p.DialContext(dialCtx, "tcp", p.RemoteAddr)
	if err != nil {
		if p.OnError != nil {
			p.OnError(fmt.Errorf("connect remote address '%s': %w", p.RemoteAddr, err))
		}
		return
	}
	defer remoteConn.Close()

	// Bidirectional copy with proper half-close handling.
	done := make(chan error, 2)

	go func() {
		_, err := io.Copy(remoteConn, localConn)
		// Close write half of remote connection if supported.
		if hc, ok := remoteConn.(halfCloser); ok {
			hc.CloseWrite()
		}
		done <- err
	}()

	go func() {
		_, err := io.Copy(localConn, remoteConn)
		// Close write half of local connection if supported.
		if hc, ok := localConn.(halfCloser); ok {
			hc.CloseWrite()
		}
		done <- err
	}()

	// Wait for both copies to complete or context cancel.
	for i := 0; i < 2; i++ {
		select {
		case <-ctx.Done():
			// Close connections to abort ongoing copies.
			localConn.Close()
			remoteConn.Close()
			return
		case err = <-done:
			if err != nil && p.OnError != nil {
				p.OnError(fmt.Errorf("data copy: %w", err))
			}
		}
	}
}
