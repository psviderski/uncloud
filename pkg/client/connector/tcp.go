package connector

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/psviderski/uncloud/internal/grpcversion"
	"golang.org/x/net/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
)

// TCPConnector establishes a connection to the machine API through a direct TCP connection to an API endpoint.
type TCPConnector struct {
	apiAddr netip.AddrPort
}

func NewTCPConnector(apiAddr netip.AddrPort) *TCPConnector {
	return &TCPConnector{apiAddr: apiAddr}
}

func (c *TCPConnector) Connect(_ context.Context) (*grpc.ClientConn, error) {
	// Use a faster connection backoff than the default (which grows up to 120s). TCP connections are typically to
	// local Docker containers (ucind) or LAN machines where long backoff delays are unnecessary.
	backoffConfig := backoff.DefaultConfig
	backoffConfig.MaxDelay = 5 * time.Second

	conn, err := grpc.NewClient(
		c.apiAddr.String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(defaultServiceConfig),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoffConfig,
			MinConnectTimeout: 5 * time.Second,
		}),
		grpc.WithUnaryInterceptor(grpcversion.ClientUnaryInterceptor),
		grpc.WithStreamInterceptor(grpcversion.ClientStreamInterceptor),
	)
	if err != nil {
		return nil, fmt.Errorf("create machine API client: %w", err)
	}
	return conn, nil
}

func (c *TCPConnector) Dialer() (proxy.ContextDialer, error) {
	return nil, fmt.Errorf("proxy connections are not supported over a TCP connection")
}

func (c *TCPConnector) Close() error {
	return nil
}
