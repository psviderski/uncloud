package client

import (
	"context"
	"slices"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (cli *Client) InspectMachine(ctx context.Context, nameOrID string) (*pb.MachineMember, error) {
	machines, err := cli.ListMachines(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, m := range machines {
		if m.Machine.Id == nameOrID || m.Machine.Name == nameOrID {
			return m, nil
		}
	}

	return nil, api.ErrNotFound
}

// ListMachines returns a list of all machines registered in the cluster that match the filter.
func (cli *Client) ListMachines(ctx context.Context, filter *api.MachineFilter) (api.MachineMembersList, error) {
	resp, err := cli.ClusterClient.ListMachines(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}

	machines := resp.Machines

	if filter != nil {
		var matchedMachines api.MachineMembersList
		for _, m := range machines {
			if MachineMatchesFilter(m, filter) {
				matchedMachines = append(matchedMachines, m)
			}
		}
		machines = matchedMachines
	}

	return machines, nil
}

func MachineMatchesFilter(machine *pb.MachineMember, filter *api.MachineFilter) bool {
	if filter == nil {
		return true
	}

	if filter.Available && machine.State == pb.MachineMember_DOWN {
		return false
	}

	if len(filter.NamesOrIDs) > 0 {
		if !slices.ContainsFunc(filter.NamesOrIDs, func(nameOrID string) bool {
			return machine.Machine.Id == nameOrID || machine.Machine.Name == nameOrID
		}) {
			return false
		}
	}

	return true
}
