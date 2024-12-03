package client

import (
	"context"
	"google.golang.org/protobuf/types/known/emptypb"
	"uncloud/internal/machine/api/pb"
)

func (cli *Client) ListMachines(ctx context.Context) ([]*pb.MachineMember, error) {
	resp, err := cli.ClusterClient.ListMachines(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return resp.Machines, nil
}
