package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/psviderski/uncloud/api/pb"
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

// UpdateMachine updates the configuration of the machine identified by nameOrID.
func (cli *Client) UpdateMachine(
	ctx context.Context, nameOrID string, req *pb.UpdateMachineRequest,
) (*pb.MachineInfo, error) {
	ctx = ProxySingleMachineContext(ctx, nameOrID)
	resp, err := cli.MachineClient.UpdateMachine(ctx, req)
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
	req := &pb.UpdateMachineRequest{
		Name: &newName,
	}
	return cli.UpdateMachine(ctx, nameOrID, req)
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

// WaitClusterReady waits for the connected machine to be ready to serve cluster requests.
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

// WaitForStoreVersion waits until the cluster store on the target machine has reached each requested actor version
// in minVersion, with no known missing or pending transactions through those versions.
// The context controls cancellation and the deadline. An empty minVersion requires no replication.
// This method observes replication without initiating synchronisation.
//
// Corrosion may satisfy a version by applying its surviving changes or by marking it complete because its changes
// have been superseded.
//
// Waiting normally makes the captured data available on the machine. However, another write may replace some of that
// data before it arrives. Corrosion can then complete the older version without transferring the replaced data.
// If the replacement is outside the requested versions, this method can succeed while the affected data is still
// missing or outdated. This can happen during concurrent updates even when all machines are well connected.
//
// Success does not guarantee an exact snapshot or delivery of every historical value.
// Callers that require a specific record or condition should verify it after waiting.
func (cli *Client) WaitForStoreVersion(ctx context.Context, minVersion map[string]uint64) error {
	_, err := cli.MachineClient.WaitForStoreVersion(ctx, &pb.WaitForStoreVersionRequest{MinVersion: minVersion})
	return err
}
