package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/cli/cli/streams"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/docker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"os"
)

var ErrNotFound = errors.New("not found")

// Client is a client for the machine API.
type Client struct {
	connector Connector
	conn      *grpc.ClientConn

	pb.MachineClient
	pb.ClusterClient
	// Docker is a namespaced client for the Docker service to distinguish Uncloud-specific service container operations
	// from generic Docker operations.
	Docker *docker.Client
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
	c.Docker = docker.NewClient(c.conn)
	return c, nil
}

func (cli *Client) Close() error {
	return errors.Join(cli.conn.Close(), cli.connector.Close())
}

// progressOut returns an output stream for progress writer.
func (cli *Client) progressOut() *streams.Out {
	return streams.NewOut(os.Stdout)
}

// proxyToMachine returns a new context that proxies gRPC requests to the specified machine.
func proxyToMachine(ctx context.Context, machine *pb.MachineInfo) context.Context {
	machineIP, _ := machine.Network.ManagementIp.ToAddr()
	md := metadata.Pairs("machines", machineIP.String())
	return metadata.NewOutgoingContext(ctx, md)
}
