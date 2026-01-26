package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
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

// MultiMachineContext is a context wrapper that includes information about the machines
// being targeted by a proxied gRPC request.
type MultiMachineContext struct {
	context.Context
	machines     api.MachineMembersList
	machinesByIP map[string]*pb.MachineMember
}

// Machines returns the list of machines targeted by this context.
func (m *MultiMachineContext) Machines() api.MachineMembersList {
	return m.machines
}

// ResolveMachine returns the machine member that corresponds to the provided metadata.
func (m *MultiMachineContext) ResolveMachine(metadata *pb.Metadata) (*pb.MachineMember, error) {
	if metadata == nil {
		if len(m.machines) == 1 {
			return m.machines[0], nil
		}
		return nil, fmt.Errorf("metadata is missing for a machine response")
	}

	if machine, ok := m.machinesByIP[metadata.Machine]; ok {
		return machine, nil
	}

	return nil, fmt.Errorf("machine not found by management IP: %s", metadata.Machine)
}

// ProxyMachinesContext returns a new context that proxies gRPC requests to the specified machines.
// If namesOrIDs is nil, all machines are included.
func (cli *Client) ProxyMachinesContext(
	ctx context.Context, namesOrIDs []string,
) (*MultiMachineContext, error) {
	// TODO: move the machine IP resolution to the proxy router to allow setting machine names and IDs in the metadata.
	machines, err := cli.ListMachines(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
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
		return nil, fmt.Errorf("machines not found: %s", strings.Join(notFound, ", "))
	}

	if len(namesOrIDs) == 0 {
		proxiedMachines = machines
	}

	md := metadata.New(nil)
	machinesByIP := make(map[string]*pb.MachineMember)
	for _, m := range proxiedMachines {
		machineIP, _ := m.Machine.Network.ManagementIp.ToAddr()
		md.Append("machines", machineIP.String())
		machinesByIP[machineIP.String()] = m
	}

	return &MultiMachineContext{
		Context:      metadata.NewOutgoingContext(ctx, md),
		machines:     proxiedMachines,
		machinesByIP: machinesByIP,
	}, nil
}

// ResolvedResult wraps a response item with the resolved machine information.
type ResolvedResult[T any] struct {
	Item        T
	Machine     *pb.MachineMember
	MachineName string
	// MachineAddr is the machine's management IP address from the response metadata.
	MachineAddr string
}

// ResolveMachines iterates over a slice of results and yields each result paired with its resolved machine.
// It uses reflection to find the metadata in the result items (looking for a Metadata field or GetMetadata method).
func ResolveMachines[T any](
	mctx *MultiMachineContext, results []T,
) func(func(ResolvedResult[T]) bool) {
	return func(yield func(ResolvedResult[T]) bool) {
		for _, res := range results {
			var metadata *pb.Metadata

			// Use reflection to find metadata to avoid forcing all types to implement an interface.
			// Check for Metadata field first.
			v := reflect.ValueOf(res)
			if v.Kind() == reflect.Ptr {
				v = v.Elem()
			}
			if v.Kind() == reflect.Struct {
				f := v.FieldByName("Metadata")
				if f.IsValid() && !f.IsNil() && f.Type() == reflect.TypeOf(&pb.Metadata{}) {
					metadata = f.Interface().(*pb.Metadata)
				}
			}

			// If field not found or nil (and struct might be just hiding it), try GetMetadata method.
			if metadata == nil {
				if getter, ok := any(res).(interface{ GetMetadata() *pb.Metadata }); ok {
					metadata = getter.GetMetadata()
				}
			}

			machine, resolveErr := mctx.ResolveMachine(metadata)
			machineName := "unknown"
			machineAddr := ""

			if metadata != nil {
				machineAddr = metadata.Machine
			}

			if machine != nil {
				machineName = machine.Machine.Name
			} else if machineAddr != "" {
				machineName = machineAddr
			}

			// Check for machine errors in metadata
			if metadata != nil && metadata.Error != "" {
				PrintWarning(fmt.Sprintf("failed to list items on machine %s: %s", machineName, metadata.Error))
				continue
			}

			// If resolution failed and we couldn't determine the machine, we might want to skip or yield with nil machine.
			// Current existing logic in ps.go handles "unknown" machineName.
			// However, if metadata is nil and we have multiple machines, ResolveMachine returns error.
			// If metadata is nil and we have 1 machine, ResolveMachine returns it.

			if resolveErr != nil && metadata == nil {
				// Ambiguous case (multiple machines but no metadata).
				// This usually indicates a proxy error.
				// We log a warning and fallback to "unknown" instead of failing the whole request.
				PrintWarning("something went wrong with gRPC proxy: metadata is missing for a machine response")
				if !yield(ResolvedResult[T]{
					Item:        res,
					Machine:     nil,
					MachineName: "unknown",
					MachineAddr: "",
				}) {
					return
				}
				continue
			}

			if !yield(ResolvedResult[T]{
				Item:        res,
				Machine:     machine,
				MachineName: machineName,
				MachineAddr: machineAddr,
			}) {
				return
			}
		}
	}
}
