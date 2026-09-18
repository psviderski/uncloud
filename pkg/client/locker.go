package client

import (
	"context"
	"fmt"
	"time"

	"github.com/psviderski/uncloud/pkg/distlock"
	distlockgrpc "github.com/psviderski/uncloud/pkg/distlock/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
)

// NewLocker creates a distlock.Locker that acquires automatically renewed distributed leases over the machines
// in the cluster. The Locker uses the client's connection, so callers must release its active leases before closing
// the client.
func (cli *Client) NewLocker(config distlock.Config) (*distlock.Locker, error) {
	return distlock.New(&lockCluster{client: cli}, config)
}

type lockCluster struct {
	client *Client
}

var _ distlock.Cluster = (*lockCluster)(nil)

// Nodes returns a point-in-time snapshot of the registered machines in the cluster, including temporarily unavailable
// ones. The Locker calls Nodes at the start of each Acquire and retains the returned snapshot across acquisition
// retries and for the lifetime of any acquired lease.
//
// Adding or removing machines, combined with eventual replication of the machine list, can cause different
// acquisitions to use different snapshots while active leases continue using older ones. This adapter does not
// version snapshots or coordinate membership transitions. If old and new snapshots allow disjoint quorums, two
// clients can acquire leases for the same resource. Membership changes must preserve quorum overlap while leases from
// older snapshots may remain valid.
func (c *lockCluster) Nodes(ctx context.Context) ([]distlock.Node, error) {
	machines, err := c.client.ListMachines(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}

	nodes := make([]distlock.Node, 0, len(machines))
	for _, m := range machines {
		nodes = append(nodes, &lockNode{
			id:     m.Machine.Id,
			leases: c.client.leases,
		})
	}
	return nodes, nil
}

type lockNode struct {
	id     string
	leases distlockgrpc.LeaseClient
}

var _ distlock.Node = (*lockNode)(nil)

func (n *lockNode) Acquire(
	ctx context.Context, resource string, token []byte, ttl time.Duration,
) (bool, error) {
	resp, err := n.leases.Acquire(ProxySingleMachineContext(ctx, n.id), &distlockgrpc.AcquireLeaseRequest{
		Resource: resource,
		Token:    token,
		Ttl:      durationpb.New(ttl),
	})
	if err != nil {
		return false, err
	}
	return resp.Acquired, nil
}

func (n *lockNode) Renew(
	ctx context.Context, resource string, token []byte, ttl time.Duration,
) (bool, error) {
	resp, err := n.leases.Renew(ProxySingleMachineContext(ctx, n.id), &distlockgrpc.RenewLeaseRequest{
		Resource: resource,
		Token:    token,
		Ttl:      durationpb.New(ttl),
	})
	if err != nil {
		return false, err
	}
	return resp.Renewed, nil
}

func (n *lockNode) Release(ctx context.Context, resource string, token []byte) (bool, error) {
	resp, err := n.leases.Release(ProxySingleMachineContext(ctx, n.id), &distlockgrpc.ReleaseLeaseRequest{
		Resource: resource,
		Token:    token,
	})
	if err != nil {
		return false, err
	}
	return resp.Released, nil
}
