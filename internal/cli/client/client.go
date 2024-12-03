package client

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/docker"
)

var ErrNotFound = errors.New("not found")

// Client is a client for the machine API.
type Client struct {
	connector Connector
	conn      *grpc.ClientConn

	pb.MachineClient
	pb.ClusterClient
	*DockerClient
}

// DockerClient is a type alias for the Docker client to embed it in Client with a more specific name.
type DockerClient = docker.Client

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
	c.DockerClient = docker.NewClient(c.conn)
	return c, nil
}

func (cli *Client) Close() error {
	return errors.Join(cli.conn.Close(), cli.connector.Close())
}
