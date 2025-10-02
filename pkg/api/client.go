package api

import (
	"context"
	"fmt"
	"slices"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"google.golang.org/grpc/metadata"
)

type Client interface {
	ContainerClient
	DNSClient
	ImageClient
	MachineClient
	ServiceClient
	VolumeClient
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
	ListMachines(ctx context.Context, filter *MachineFilter) (MachineMembersList, error)
	UpdateMachine(ctx context.Context, req *pb.UpdateMachineRequest) (*pb.MachineInfo, error)
	RenameMachine(ctx context.Context, nameOrID, newName string) (*pb.MachineInfo, error)
}

type ServiceClient interface {
	RunService(ctx context.Context, spec ServiceSpec) (RunServiceResponse, error)
	InspectService(ctx context.Context, id string) (Service, error)
	RemoveService(ctx context.Context, id string) error
}

type VolumeClient interface {
	CreateVolume(ctx context.Context, machineNameOrID string, opts volume.CreateOptions) (MachineVolume, error)
	ListVolumes(ctx context.Context, filter *VolumeFilter) ([]MachineVolume, error)
	RemoveVolume(ctx context.Context, machineNameOrID, volumeName string, force bool) error
}

// ProxyMachinesContext returns a new context that proxies gRPC requests to the specified machines.
// If namesOrIDs is nil, all machines are included.
func ProxyMachinesContext(
	ctx context.Context, cli MachineClient, namesOrIDs []string,
) (context.Context, MachineMembersList, error) {
	// TODO: move the machine IP resolution to the proxy router to allow setting machine names and IDs in the metadata.
	machines, err := cli.ListMachines(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("list machines: %w", err)
	}

	var proxiedMachines MachineMembersList
	md := metadata.New(nil)
	for _, m := range machines {
		if len(namesOrIDs) == 0 ||
			slices.Contains(namesOrIDs, m.Machine.Name) || slices.Contains(namesOrIDs, m.Machine.Id) {
			proxiedMachines = append(proxiedMachines, m)
			machineIP, _ := m.Machine.Network.ManagementIp.ToAddr()
			md.Append("machines", machineIP.String())
		}
	}

	return metadata.NewOutgoingContext(ctx, md), proxiedMachines, nil
}
