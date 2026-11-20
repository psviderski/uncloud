package hub

import (
	"context"
	"crypto/ecdh"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/hub/tunnel"
	"github.com/psviderski/uncloud/internal/secret"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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

// Status describes the tunnel of a Connector.
type Status struct {
	// Connected reports whether the tunnel is open.
	Connected bool
	// Since is when the tunnel opened or closed. It's zero if the connector hasn't tried to open it yet.
	Since time.Time
	// LastError is the error that last closed the tunnel or prevented it from opening.
	LastError error
}

// Connector keeps a tunnel from the machine to Hub open and serves Hub requests from it.
type Connector struct {
	endpoint       string
	creds          tunnel.Credentials
	target         string
	transportCreds credentials.TransportCredentials
	server         *grpc.Server

	minRetryInterval time.Duration
	maxRetryInterval time.Duration

	mu     sync.RWMutex
	status Status
}

// NewConnector creates a Connector that serves Hub requests from the tunnel with the server, for example the one
// returned by NewAPIServer. The caller owns the server and should stop it after Run returns.
func NewConnector(config Config, server *grpc.Server) (*Connector, error) {
	target, useTLS, err := parseEndpoint(config.Endpoint)
	if err != nil {
		return nil, err
	}
	if config.ClusterID == "" || config.MachineID == "" {
		return nil, errors.New("cluster and machine IDs must be set")
	}
	if _, err = ecdh.X25519().NewPrivateKey(config.PrivateKey); err != nil {
		return nil, fmt.Errorf("invalid WireGuard private key: %w", err)
	}

	transportCreds := insecure.NewCredentials()
	if useTLS {
		// TLS only verifies Hub using the system root certificates.
		transportCreds = credentials.NewClientTLSFromCert(nil, "")
	}

	return &Connector{
		endpoint: config.Endpoint,
		creds: tunnel.Credentials{
			ClusterID:  config.ClusterID,
			MachineID:  config.MachineID,
			PrivateKey: config.PrivateKey,
		},
		target:           target,
		transportCreds:   transportCreds,
		server:           server,
		minRetryInterval: defaultMinRetryInterval,
		maxRetryInterval: defaultMaxRetryInterval,
	}, nil
}

// parseEndpoint returns the gRPC target to dial for the Hub endpoint and whether to use TLS. The endpoint must be
// an http or https URL without a path.
func parseEndpoint(endpoint string) (target string, useTLS bool, err error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", false, fmt.Errorf("parse Hub endpoint: %w", err)
	}

	port := u.Port()
	switch strings.ToLower(u.Scheme) {
	case "https":
		useTLS = true
		if port == "" {
			port = "443"
		}
	case "http":
		if port == "" {
			port = "80"
		}
	default:
		return "", false, fmt.Errorf("invalid Hub endpoint '%s': scheme must be http or https", endpoint)
	}

	if u.Hostname() == "" {
		return "", false, fmt.Errorf("invalid Hub endpoint '%s': host is empty", endpoint)
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", false, fmt.Errorf("invalid Hub endpoint '%s': only scheme, host and port are allowed", endpoint)
	}

	return net.JoinHostPort(u.Hostname(), port), useTLS, nil
}

// Status returns the current status of the tunnel.
func (c *Connector) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()

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

	defer c.setStatus(false, nil)

	boff := backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(c.minRetryInterval),
		backoff.WithMaxInterval(c.maxRetryInterval),
		backoff.WithMaxElapsedTime(0),
	)
	for {
		conn, err := c.open(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.setStatus(false, err)

			wait := boff.NextBackOff()
			slog.Warn("Failed to open tunnel to Hub.", "endpoint", c.endpoint, "retry_in", wait, "err", err)
			if !sleep(ctx, wait) {
				return nil
			}
			continue
		}

		boff.Reset()
		c.setStatus(true, nil)
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

		err = conn.Err()
		c.setStatus(false, err)
		// Wait before reopening to avoid a tight loop if Hub keeps closing tunnels right after accepting them.
		wait := boff.NextBackOff()
		slog.Warn("Tunnel to Hub closed.", "endpoint", c.endpoint, "retry_in", wait, "err", err)
		if !sleep(ctx, wait) {
			return nil
		}
	}
}

// open opens a tunnel to Hub over a new client connection that closes when the tunnel ends. The API server ends the
// tunnel when Hub stops answering its pings, see NewAPIServer.
func (c *Connector) open(ctx context.Context) (*tunnel.Conn, error) {
	cc, err := grpc.NewClient(c.target, grpc.WithTransportCredentials(c.transportCreds))
	if err != nil {
		return nil, fmt.Errorf("create Hub client: %w", err)
	}

	conn, err := tunnel.Open(ctx, pb.NewHubClient(cc), c.creds)
	if err != nil {
		cc.Close()
		return nil, err
	}

	go func() {
		<-conn.Done()
		cc.Close()
	}()

	return conn, nil
}

func (c *Connector) setStatus(connected bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.status.Connected != connected || c.status.Since.IsZero() {
		c.status.Since = time.Now()
	}
	c.status.Connected = connected
	if err != nil {
		c.status.LastError = err
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
