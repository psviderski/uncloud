package client

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"uncloud/internal/machine/api/pb"
)

type Client struct {
	connector Connector
	conn      *grpc.ClientConn

	pb.MachineClient
}

// Connector is an interface for establishing a connection to the machine API.
type Connector interface {
	Connect(ctx context.Context) (*grpc.ClientConn, error)
	Close() error
}

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
	return c, nil
}

func (c *Client) Close() error {
	err := c.conn.Close()
	return errors.Join(err, c.connector.Close())
}
