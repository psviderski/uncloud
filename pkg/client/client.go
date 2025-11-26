package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/docker/cli/cli/streams"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/pkg/api"
	"golang.org/x/net/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Client is a client for the machine API.
type Client struct {
	connector Connector
	conn      *grpc.ClientConn

	// TODO: refactor to not embed MachineClient and instead expose only required methods.
	//  Methods such as Reset or Inspect are ambiguous in the context of a machine+cluster client.
	pb.MachineClient
	pb.ClusterClient
	Caddy pb.CaddyClient
	// Docker is a namespaced client for the Docker service to distinguish Uncloud-specific service container operations
	// from generic Docker operations.
	Docker *docker.Client
}

var _ api.Client = (*Client)(nil)

// Connector is an interface for establishing a connection to the cluster.
type Connector interface {
	// Connect establishes a gRPC client connection to the machine API.
	Connect(ctx context.Context) (*grpc.ClientConn, error)
	// Dialer returns a proxy dialer for establishing connections within the cluster if supported by the connector.
	Dialer() (proxy.ContextDialer, error)
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
	c.Caddy = pb.NewCaddyClient(c.conn)
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

// ProxyMachinesContext returns a new context that proxies gRPC requests to the specified machines.
// If namesOrIDs is nil, all machines are included.
func (cli *Client) ProxyMachinesContext(
	ctx context.Context, namesOrIDs []string,
) (context.Context, api.MachineMembersList, error) {
	// TODO: move the machine IP resolution to the proxy router to allow setting machine names and IDs in the metadata.
	machines, err := cli.ListMachines(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("list machines: %w", err)
	}

	var proxiedMachines api.MachineMembersList
	var notFound []string
	for _, nameOrID := range namesOrIDs {
		if m := machines.FindByNameOrID(nameOrID); m != nil {
			proxiedMachines = append(proxiedMachines, m)
		} else {
			notFound = append(notFound, nameOrID)
		}
	}

	if len(notFound) > 0 {
		return nil, nil, fmt.Errorf("machines not found: %s", strings.Join(notFound, ", "))
	}

	if len(namesOrIDs) == 0 {
		proxiedMachines = machines
	}

	md := metadata.New(nil)
	for _, m := range proxiedMachines {
		machineIP, _ := m.Machine.Network.ManagementIp.ToAddr()
		md.Append("machines", machineIP.String())
	}

	return metadata.NewOutgoingContext(ctx, md), proxiedMachines, nil
}
