package connector

import (
	"context"
	"fmt"
	"net/netip"

	"google.golang.org/grpc"
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
	conn, err := grpc.NewClient(
		c.apiAddr.String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("create machine API client: %w", err)
	}
	return conn, nil
}

func (c *TCPConnector) Close() error {
	return nil
}
