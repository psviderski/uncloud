package api

import (
	"context"
	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

type Client interface {
	ContainerClient
	DNSClient
	MachineClient
	ServiceClient
}

type ContainerClient interface {
	CreateContainer(
		ctx context.Context, serviceID string, spec ServiceSpec, machineID string,
	) (container.CreateResponse, error)
	InspectContainer(ctx context.Context, serviceID, containerID string) (MachineContainer, error)
	RemoveContainer(ctx context.Context, serviceID, containerID string, opts container.RemoveOptions) error
	StartContainer(ctx context.Context, serviceID, containerID string) error
	StopContainer(ctx context.Context, serviceID, containerID string, opts container.StopOptions) error
}

type DNSClient interface {
	GetDomain(ctx context.Context) (string, error)
}

type MachineClient interface {
	InspectMachine(ctx context.Context, id string) (*pb.MachineMember, error)
	ListMachines(ctx context.Context) ([]*pb.MachineMember, error)
}

type ServiceClient interface {
	InspectService(ctx context.Context, id string) (Service, error)
}
