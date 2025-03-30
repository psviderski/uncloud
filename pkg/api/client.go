package api

import (
	"context"
	"fmt"
	"slices"

	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"google.golang.org/grpc/metadata"
)

type Client interface {
	ContainerClient
	DNSClient
	ImageClient
	MachineClient
	ServiceClient
}

type ContainerClient interface {
	CreateContainer(
		ctx context.Context, serviceID string, spec ServiceSpec, machineID string,
	) (container.CreateResponse, error)
	InspectContainer(ctx context.Context, serviceNameOrID, containerNameOrID string) (MachineServiceContainer, error)
	RemoveContainer(ctx context.Context, serviceNameOrID, containerNameOrID string, opts container.RemoveOptions) error
	StartContainer(ctx context.Context, serviceNameOrID, containerNameOrID string) error
	StopContainer(ctx context.Context, serviceNameOrID, containerNameOrID string, opts container.StopOptions) error
}

type DNSClient interface {
	GetDomain(ctx context.Context) (string, error)
}

type ImageClient interface {
	InspectImage(ctx context.Context, id string) ([]MachineImage, error)
	InspectRemoteImage(ctx context.Context, id string) ([]MachineRemoteImage, error)
}

type MachineClient interface {
	InspectMachine(ctx context.Context, id string) (*pb.MachineMember, error)
	ListMachines(ctx context.Context) ([]*pb.MachineMember, error)
}

type ServiceClient interface {
	InspectService(ctx context.Context, id string) (Service, error)
}

// ProxyMachinesContext returns a new context that proxies gRPC requests to the specified machines.
// If namesOrIDs is nil, all machines are included.
func ProxyMachinesContext(ctx context.Context, cli MachineClient, namesOrIDs []string) (context.Context, error) {
	machines, err := cli.ListMachines(ctx)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}

	md := metadata.New(nil)
	for _, m := range machines {
		if namesOrIDs == nil ||
			slices.Contains(namesOrIDs, m.Machine.Name) || slices.Contains(namesOrIDs, m.Machine.Id) {
			machineIP, _ := m.Machine.Network.ManagementIp.ToAddr()
			md.Append("machines", machineIP.String())
		}
	}

	return metadata.NewOutgoingContext(ctx, md), nil
}
