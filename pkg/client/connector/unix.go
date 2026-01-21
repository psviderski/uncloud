package connector

import (
	"context"
	"fmt"

	"golang.org/x/net/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// UnixConnector establishes a connection to the machine API through a unix domain socket.
type UnixConnector struct {
	socketPath string
}

func NewUnixConnector(socketPath string) *UnixConnector {
	return &UnixConnector{socketPath: socketPath}
}

func (c *UnixConnector) Connect(_ context.Context) (*grpc.ClientConn, error) {
	// gRPC uses "unix:path" syntax for unix sockets.
	target := "unix:" + c.socketPath

	conn, err := grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(defaultServiceConfig),
	)
	if err != nil {
		return nil, fmt.Errorf("create machine API client: %w", err)
	}
	return conn, nil
}

func (c *UnixConnector) Dialer() (proxy.ContextDialer, error) {
	return nil, fmt.Errorf("proxy connections are not supported over a unix connection")
}

func (c *UnixConnector) Close() error {
	return nil
}
