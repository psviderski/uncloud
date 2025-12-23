package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (cli *Client) InspectMachine(ctx context.Context, nameOrID string) (*pb.MachineMember, error) {
	// TODO: refactor to use MachineClient.InspectMachine.
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
	machines := api.MachineMembersList(resp.Machines)

	if filter == nil {
		return machines, nil
	}

	// Apply the filter.
	if len(filter.NamesOrIDs) > 0 {
		var matched api.MachineMembersList
		var notFound []string

		for _, nameOrID := range filter.NamesOrIDs {
			if m := machines.FindByNameOrID(nameOrID); m != nil {
				matched = append(matched, m)
			} else {
				notFound = append(notFound, nameOrID)
			}
		}
		machines = matched

		if len(notFound) > 0 {
			return nil, fmt.Errorf("machines not found: %s", strings.Join(notFound, ", "))
		}
	}

	if filter.Available {
		var available api.MachineMembersList
		for _, m := range machines {
			if m.State != pb.MachineMember_DOWN {
				available = append(available, m)
			}
		}
		machines = available
	}

	return machines, nil
}

// UpdateMachine updates machine configuration in the cluster.
func (cli *Client) UpdateMachine(ctx context.Context, req *pb.UpdateMachineRequest) (*pb.MachineInfo, error) {
	resp, err := cli.ClusterClient.UpdateMachine(ctx, req)
	if err != nil {
		if s, ok := status.FromError(err); ok && s.Code() == codes.NotFound {
			return nil, api.ErrNotFound
		}
		return nil, err
	}
	return resp.Machine, nil
}

// RenameMachine renames an existing machine in the cluster.
func (cli *Client) RenameMachine(ctx context.Context, nameOrID, newName string) (*pb.MachineInfo, error) {
	// First, resolve the machine to get its ID
	machine, err := cli.InspectMachine(ctx, nameOrID)
	if err != nil {
		return nil, err
	}

	// Update the machine with the new name
	req := &pb.UpdateMachineRequest{
		MachineId: machine.Machine.Id,
		Name:      &newName,
	}

	return cli.UpdateMachine(ctx, req)
}

// WaitMachineReady waits for the machine API on the connected machine to respond.
func (cli *Client) WaitMachineReady(ctx context.Context, timeout time.Duration) error {
	boff := backoff.WithContext(backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(100*time.Millisecond),
		backoff.WithMaxInterval(1*time.Second),
		backoff.WithMaxElapsedTime(timeout),
	), ctx)

	inspect := func() error {
		if _, err := cli.Inspect(ctx, &emptypb.Empty{}); err != nil {
			return fmt.Errorf("inspect machine: %w", err)
		}
		return nil
	}
	return backoff.Retry(inspect, boff)
}

// WaitClusterReady waits for the connected machine to be ready to server cluster requests.
func (cli *Client) WaitClusterReady(ctx context.Context, timeout time.Duration) error {
	// Backoff is not really needed here as the default service config for the gRPC client is already
	// doing retries with backoff for Unavailable errors. However, it's still convenient to use backoff
	// to control the overall timeout for the operation.
	boff := backoff.WithContext(backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(100*time.Millisecond),
		backoff.WithMaxInterval(1*time.Second),
		backoff.WithMaxElapsedTime(timeout),
	), ctx)

	listMachines := func() error {
		_, err := cli.ListMachines(ctx, nil)
		if err != nil {
			if s, ok := status.FromError(err); ok &&
				// TODO: remove FailedPrecondition after releading 0.17.
				(s.Code() == codes.Unavailable || s.Code() == codes.FailedPrecondition) {
				// Machine is not ready yet, retry.
				return err
			}
			// Other non-Unavailable errors should not be retried.
			return backoff.Permanent(err)
		}
		return nil
	}
	return backoff.Retry(listMachines, boff)
}
