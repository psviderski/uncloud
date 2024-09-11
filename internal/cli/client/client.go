package client

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"uncloud/internal/machine/api/pb"
)

// Client is a client for the machine API.
type Client struct {
	connector Connector
	conn      *grpc.ClientConn

	pb.MachineClient
	pb.ClusterClient
}

// Connector is an interface for establishing a connection to the machine API.
type Connector interface {
	Connect(ctx context.Context) (*grpc.ClientConn, error)
	Close() error
}

// New creates a new client for the machine API. The connector is used to establish the connection
// either locally or remotely. The client is responsible for closing the connector.
func New(ctx context.Context, connector Connector) (*Client, error) {
	c := &Client{
		connector: connector,
	}
	var err error
	c.conn, err = connector.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect to machine: %w", err)
	}

	c.MachineClient = pb.NewMachineClient(c.conn)
	c.ClusterClient = pb.NewClusterClient(c.conn)
	return c, nil
}

func (c *Client) Close() error {
	err := c.conn.Close()
	return errors.Join(err, c.connector.Close())
}
