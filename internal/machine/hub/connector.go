package hub

import (
	"context"
	"crypto/ecdh"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/psviderski/uncloud/hub/tunnel"
	"github.com/psviderski/uncloud/internal/secret"
	"google.golang.org/grpc"
)

const (
	defaultMinRetryInterval = time.Second
	defaultMaxRetryInterval = time.Minute
)

// Config configures a Connector.
type Config struct {
	// Endpoint is the Hub gRPC API endpoint that the machine opens tunnels to. An https endpoint uses TLS with
	// the system root certificates. An http endpoint uses plaintext and is only meant for development.
	Endpoint  string
	ClusterID string
	MachineID string
	// PrivateKey is the WireGuard private key of the machine that authenticates it to Hub.
	PrivateKey secret.Secret
}

// State is the state of the tunnel from a machine to Hub.
type State string

const (
	// StateInactive means the machine doesn't open a tunnel.
	StateInactive State = "inactive"
	// StateConnecting means the machine is opening the tunnel.
	StateConnecting State = "connecting"
	// StateConnected means the tunnel is open.
	StateConnected State = "connected"
	// StateRetrying means the tunnel couldn't be opened or has closed, and the machine waits to retry.
	StateRetrying State = "retrying"
	// StateFailed means the machine can't open the tunnel and doesn't retry.
	StateFailed State = "failed"
)

// Status describes the tunnel from a machine to Hub.
type Status struct {
	State State
	// Since is when the tunnel opened if it's connected. Otherwise, it's when the tunnel closed or the machine started
	// opening it, or when it entered the inactive or failed state. It doesn't change while the machine retries.
	Since time.Time
	// RetryAt is when the machine retries to open the tunnel. It's only set in the retrying state.
	RetryAt time.Time
	// LastError is the error that last closed the tunnel or prevented it from opening. It's kept after the tunnel
	// opens again.
	LastError   error
	LastErrorAt time.Time
}

// next returns the status after the transition to state at now caused by err, if any.
func (s Status) next(state State, now time.Time, err error, retryAt time.Time) Status {
	// Since doesn't change while the machine alternates between connecting and retrying because it can't open
	// the tunnel. This way, it shows how long the tunnel has been down.
	opening := (s.State == StateConnecting || s.State == StateRetrying) &&
		(state == StateConnecting || state == StateRetrying)
	if s.Since.IsZero() || (s.State != state && !opening) {
		s.Since = now
	}
	s.State = state
	s.RetryAt = retryAt
	if err != nil {
		s.LastError = err
		s.LastErrorAt = now
	}
	return s
}

// Connector keeps a tunnel from the machine to Hub open and serves Hub requests from it.
type Connector struct {
	endpoint string
	creds    tunnel.Credentials
	server   *grpc.Server

	minRetryInterval time.Duration
	maxRetryInterval time.Duration

	mu     sync.Mutex
	status Status
	// onStatus is called with the new status after every change. It's called from the Run goroutine, so the calls
	// are ordered and none happen after Run returns.
	onStatus func(Status)
}

// NewConnector creates a Connector that serves Hub requests from the tunnel with the server, for example the one
// returned by NewAPIServer. The caller owns the server and should stop it after Run returns.
func NewConnector(config Config, server *grpc.Server) (*Connector, error) {
	if _, _, err := parseEndpoint(config.Endpoint); err != nil {
		return nil, err
	}
	if config.ClusterID == "" || config.MachineID == "" {
		return nil, errors.New("cluster and machine IDs must be set")
	}
	if _, err := ecdh.X25519().NewPrivateKey(config.PrivateKey); err != nil {
		return nil, fmt.Errorf("invalid WireGuard private key: %w", err)
	}

	return &Connector{
		endpoint: config.Endpoint,
		creds: tunnel.Credentials{
			ClusterID:  config.ClusterID,
			MachineID:  config.MachineID,
			PrivateKey: config.PrivateKey,
		},
		server:           server,
		minRetryInterval: defaultMinRetryInterval,
		maxRetryInterval: defaultMaxRetryInterval,
		status:           Status{State: StateInactive},
	}, nil
}

// Status returns the current status of the tunnel.
func (c *Connector) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.status
}

// Run keeps a tunnel to Hub open until ctx is cancelled and serves Hub requests from it. It reopens the tunnel
// with exponential backoff when it closes or can't be opened. It returns an error only if the server stops serving.
func (c *Connector) Run(ctx context.Context) error {
	// The server closes the listener when it stops serving. The connector can't hand it tunnels after that.
	listener := newConnListener()
	serveDone := make(chan struct{})

	go func() {
		defer close(serveDone)
		if sErr := c.server.Serve(listener); sErr != nil && !errors.Is(sErr, net.ErrClosed) {
			slog.Error("Hub API server failed.", "err", sErr)
		}
	}()
	defer func() {
		listener.Close()
		<-serveDone
	}()

	defer c.setStatus(StateInactive, nil, time.Time{})

	boff := backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(c.minRetryInterval),
		backoff.WithMaxInterval(c.maxRetryInterval),
		backoff.WithMaxElapsedTime(0),
	)
	for {
		c.setStatus(StateConnecting, nil, time.Time{})
		conn, err := c.open(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			wait := boff.NextBackOff()
			c.setStatus(StateRetrying, err, time.Now().Add(wait))
			slog.Warn("Failed to open tunnel to Hub.", "endpoint", c.endpoint, "retry_in", wait, "err", err)
			if !sleep(ctx, wait) {
				return nil
			}
			continue
		}

		boff.Reset()
		c.setStatus(StateConnected, nil, time.Time{})
		slog.Info("Tunnel to Hub opened.", "endpoint", c.endpoint)

		if err = listener.push(ctx, conn); err != nil {
			conn.Close()
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("Hub API server stopped serving: %w", err)
		}

		select {
		case <-conn.Done():
		case <-ctx.Done():
			conn.Close()
			return nil
		}

		err = fmt.Errorf("tunnel closed: %w", conn.Err())
		// Wait before reopening to avoid a tight loop if Hub keeps closing tunnels right after accepting them.
		wait := boff.NextBackOff()
		c.setStatus(StateRetrying, err, time.Now().Add(wait))
		slog.Warn("Tunnel to Hub closed.", "endpoint", c.endpoint, "retry_in", wait, "err", err)
		if !sleep(ctx, wait) {
			return nil
		}
	}
}

// open opens a tunnel to Hub over a new client connection that closes when the tunnel ends. The API server ends the
// tunnel when Hub stops answering its pings, see NewAPIServer.
func (c *Connector) open(ctx context.Context) (*tunnel.Conn, error) {
	client, err := NewClient(c.endpoint)
	if err != nil {
		return nil, fmt.Errorf("create Hub client: %w", err)
	}

	conn, err := tunnel.Open(ctx, client, c.creds)
	if err != nil {
		client.Close()
		return nil, err
	}

	go func() {
		<-conn.Done()
		client.Close()
	}()

	return conn, nil
}

func (c *Connector) setStatus(state State, err error, retryAt time.Time) {
	c.mu.Lock()
	c.status = c.status.next(state, time.Now(), err, retryAt)
	status := c.status
	onStatus := c.onStatus
	c.mu.Unlock()

	if onStatus != nil {
		onStatus(status)
	}
}

// sleep waits for d and returns false if ctx is cancelled first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
