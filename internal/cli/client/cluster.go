package client

import (
	"context"
	"google.golang.org/protobuf/types/known/emptypb"
	"uncloud/internal/machine/api/pb"
)

func (cli *Client) InspectMachine(ctx context.Context, id string) (*pb.MachineMember, error) {
	machines, err := cli.ListMachines(ctx)
	if err != nil {
		return nil, err
	}

	for _, m := range machines {
		if m.Machine.Id == id || m.Machine.Name == id {
			return m, nil
		}
	}

	return nil, ErrNotFound
}

func (cli *Client) ListMachines(ctx context.Context) ([]*pb.MachineMember, error) {
	resp, err := cli.ClusterClient.ListMachines(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return resp.Machines, nil
}
