package distlock_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/psviderski/uncloud/pkg/distlock"
	"github.com/stretchr/testify/require"
)

var errMemoryNodeUnavailable = errors.New("memory node unavailable")

type memoryCluster struct {
	nodes []*memoryNode
}

func newMemoryCluster(size int) *memoryCluster {
	cluster := &memoryCluster{nodes: make([]*memoryNode, size)}
	for i := range cluster.nodes {
		cluster.nodes[i] = &memoryNode{store: distlock.NewMemoryStore(), available: true}
	}
	return cluster
}

func (c *memoryCluster) Nodes(ctx context.Context) ([]distlock.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	nodes := make([]distlock.Node, len(c.nodes))
	for i, node := range c.nodes {
		nodes[i] = node
	}
	return nodes, nil
}

type memoryNode struct {
	mu                  sync.Mutex
	store               distlock.Store
	available           bool
	unavailableObserved chan struct{}
}

func (n *memoryNode) setAvailable(available bool) {
	if !available {
		n.makeUnavailable()
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	n.available = true
}

func (n *memoryNode) makeUnavailable() <-chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.available = false
	n.unavailableObserved = make(chan struct{})
	return n.unavailableObserved
}

func (n *memoryNode) restart() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.store = distlock.NewMemoryStore()
}

func (n *memoryNode) currentStore(ctx context.Context) (distlock.Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.available {
		if n.unavailableObserved != nil {
			close(n.unavailableObserved)
			n.unavailableObserved = nil
		}
		return nil, errMemoryNodeUnavailable
	}
	return n.store, nil
}

func (n *memoryNode) Acquire(
	ctx context.Context, resource string, token []byte, ttl time.Duration,
) (bool, error) {
	store, err := n.currentStore(ctx)
	if err != nil {
		return false, err
	}
	return store.Acquire(ctx, resource, token, ttl)
}

func (n *memoryNode) Renew(
	ctx context.Context, resource string, token []byte, ttl time.Duration,
) (bool, error) {
	store, err := n.currentStore(ctx)
	if err != nil {
		return false, err
	}
	return store.Renew(ctx, resource, token, ttl)
}

func (n *memoryNode) Release(ctx context.Context, resource string, token []byte) (bool, error) {
	store, err := n.currentStore(ctx)
	if err != nil {
		return false, err
	}
	return store.Release(ctx, resource, token)
}

func retryBackOff() backoff.BackOff {
	return backoff.NewConstantBackOff(5 * time.Millisecond)
}

func oneAttemptBackOff() backoff.BackOff {
	return &backoff.StopBackOff{}
}

func newTestLocker(
	t *testing.T, cluster distlock.Cluster, leaseDuration time.Duration, newBackOff func() backoff.BackOff,
) *distlock.Locker {
	t.Helper()
	locker, err := distlock.New(cluster, distlock.Config{
		LeaseDuration:   leaseDuration,
		NodeCallTimeout: leaseDuration / 3,
		NewBackOff:      newBackOff,
	})
	require.NoError(t, err)
	return locker
}

func acquireLease(t *testing.T, locker *distlock.Locker, resource string) *distlock.Lease {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	lease, err := locker.Acquire(ctx, resource)
	require.NoError(t, err)
	return lease
}

func releaseLease(t *testing.T, lease *distlock.Lease) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, lease.Release(ctx))
}

func requireNoReceive[T any](t *testing.T, ch <-chan T, timeout time.Duration, message string) {
	t.Helper()
	select {
	case <-ch:
		require.FailNow(t, message)
	case <-time.After(timeout):
	}
}

func requireReceive[T any](t *testing.T, ch <-chan T, timeout time.Duration, message string) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(timeout):
		require.FailNow(t, message)
		var zero T
		return zero
	}
}

func requireContextActive(t *testing.T, ctx context.Context, message string) {
	t.Helper()
	select {
	case <-ctx.Done():
		require.FailNow(t, message, "cause: %v", context.Cause(ctx))
	default:
	}
}

func requireSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration, message string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		require.FailNow(t, message)
	}
}

func TestLockerAcquireQuorumBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		nodes       int
		unavailable int
		wantAcquire bool
	}{
		{name: "two nodes at quorum", nodes: 2, wantAcquire: true},
		{name: "two nodes below quorum", nodes: 2, unavailable: 1, wantAcquire: false},
		{name: "three nodes at quorum", nodes: 3, unavailable: 1, wantAcquire: true},
		{name: "three nodes below quorum", nodes: 3, unavailable: 2, wantAcquire: false},
		{name: "four nodes at quorum", nodes: 4, unavailable: 1, wantAcquire: true},
		{name: "four nodes below quorum", nodes: 4, unavailable: 2, wantAcquire: false},
		{name: "five nodes at quorum", nodes: 5, unavailable: 2, wantAcquire: true},
		{name: "five nodes below quorum", nodes: 5, unavailable: 3, wantAcquire: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newMemoryCluster(tt.nodes)
			unavailableObserved := make([]<-chan struct{}, 0, tt.unavailable)
			for i := range tt.unavailable {
				unavailableObserved = append(unavailableObserved, cluster.nodes[tt.nodes-1-i].makeUnavailable())
			}
			locker := newTestLocker(t, cluster, 300*time.Millisecond, oneAttemptBackOff)

			acquireCtx, cancelAcquire := context.WithTimeout(context.Background(), 2*time.Second)
			lease, err := locker.Acquire(acquireCtx, "resource")
			cancelAcquire()
			for _, observed := range unavailableObserved {
				requireSignal(t, observed, time.Second, "unavailable node did not receive acquisition")
			}
			for i := range tt.unavailable {
				cluster.nodes[tt.nodes-1-i].setAvailable(true)
			}

			if tt.wantAcquire {
				require.NoError(t, err)
				require.NotNil(t, lease)
				releaseLease(t, lease)
			} else {
				if lease != nil {
					releaseLease(t, lease)
				}
				require.Error(t, err)
				require.Nil(t, lease)
			}
		})
	}
}

func TestLockerSingleNodeLifecycle(t *testing.T) {
	const leaseDuration = 300 * time.Millisecond
	cluster := newMemoryCluster(1)
	locker := newTestLocker(t, cluster, leaseDuration, oneAttemptBackOff)

	lease := acquireLease(t, locker, "resource")
	defer func() {
		if lease.Context().Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = lease.Release(ctx)
	}()

	time.Sleep(leaseDuration + 100*time.Millisecond)
	requireContextActive(t, lease.Context(), "single-node lease was not renewed")
	releaseLease(t, lease)

	secondLease := acquireLease(t, locker, "resource")
	releaseLease(t, secondLease)
}

func TestLockerFailedAcquireCleansUpPartialLease(t *testing.T) {
	cluster := newMemoryCluster(3)
	cluster.nodes[1].setAvailable(false)
	cluster.nodes[2].setAvailable(false)
	locker := newTestLocker(t, cluster, 300*time.Millisecond, oneAttemptBackOff)

	lease, err := locker.Acquire(context.Background(), "resource")
	require.Error(t, err)
	require.Nil(t, lease)

	cluster.nodes[1].setAvailable(true)
	cluster.nodes[2].setAvailable(true)
	lease = acquireLease(t, locker, "resource")
	releaseLease(t, lease)
}

func TestLockerAcquiresIndependentResourcesConcurrently(t *testing.T) {
	cluster := newMemoryCluster(3)
	locker := newTestLocker(t, cluster, 300*time.Millisecond, oneAttemptBackOff)
	resources := []string{"database", "deployment", "network", "volume"}

	type acquireResult struct {
		resource string
		lease    *distlock.Lease
		err      error
	}
	resultCh := make(chan acquireResult, len(resources))
	start := make(chan struct{})
	acquireCtx, cancelAcquire := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelAcquire()
	for _, resource := range resources {
		go func() {
			<-start
			lease, err := locker.Acquire(acquireCtx, resource)
			resultCh <- acquireResult{resource: resource, lease: lease, err: err}
		}()
	}
	close(start)

	results := make([]acquireResult, 0, len(resources))
	leases := make([]*distlock.Lease, 0, len(resources))
	for range resources {
		result := requireReceive(t, resultCh, 2*time.Second, "concurrent acquisition did not finish")
		results = append(results, result)
		if result.lease != nil {
			leases = append(leases, result.lease)
		}
	}
	defer func() {
		for _, lease := range leases {
			releaseLease(t, lease)
		}
	}()

	for _, result := range results {
		require.NoErrorf(t, result.err, "acquire %q", result.resource)
		require.NotNilf(t, result.lease, "acquire %q", result.resource)
		requireContextActive(t, result.lease.Context(), "independent lease was lost")
	}
}

func TestLockerExcludesCompetingLeaseUntilRelease(t *testing.T) {
	cluster := newMemoryCluster(3)
	firstLocker := newTestLocker(t, cluster, 300*time.Millisecond, retryBackOff)
	secondLocker := newTestLocker(t, cluster, 300*time.Millisecond, retryBackOff)

	firstLease := acquireLease(t, firstLocker, "resource")

	type acquireResult struct {
		lease *distlock.Lease
		err   error
	}
	resultCh := make(chan acquireResult, 1)
	acquireCtx, cancelAcquire := context.WithTimeout(context.Background(), time.Second)
	defer cancelAcquire()
	go func() {
		lease, acquireErr := secondLocker.Acquire(acquireCtx, "resource")
		resultCh <- acquireResult{lease: lease, err: acquireErr}
	}()

	requireNoReceive(t, resultCh, 50*time.Millisecond, "competing acquisition returned before release")
	releaseLease(t, firstLease)
	require.ErrorIs(t, context.Cause(firstLease.Context()), distlock.ErrLeaseReleased)

	result := requireReceive(t, resultCh, time.Second, "competing acquisition did not finish after release")
	require.NoError(t, result.err)
	require.NotNil(t, result.lease)
	releaseLease(t, result.lease)
}

func TestLockerContendingAcquireRespectsContext(t *testing.T) {
	cluster := newMemoryCluster(3)
	firstLocker := newTestLocker(t, cluster, 300*time.Millisecond, retryBackOff)
	secondLocker := newTestLocker(t, cluster, 300*time.Millisecond, retryBackOff)

	firstLease := acquireLease(t, firstLocker, "resource")
	t.Cleanup(func() {
		if firstLease.Context().Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = firstLease.Release(ctx)
	})

	acquireCtx, cancelAcquire := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancelAcquire()
	competingLease, err := secondLocker.Acquire(acquireCtx, "resource")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, competingLease)
	requireContextActive(t, firstLease.Context(), "holding lease was affected by a competing acquisition")

	releaseLease(t, firstLease)
	secondLease := acquireLease(t, secondLocker, "resource")
	releaseLease(t, secondLease)
}

func TestLockerAutomaticallyRenewsLease(t *testing.T) {
	const leaseDuration = 300 * time.Millisecond
	cluster := newMemoryCluster(3)
	locker := newTestLocker(t, cluster, leaseDuration, retryBackOff)
	competingLocker := newTestLocker(t, cluster, leaseDuration, oneAttemptBackOff)

	lease := acquireLease(t, locker, "resource")
	defer releaseLease(t, lease)

	time.Sleep(leaseDuration + 100*time.Millisecond)
	requireContextActive(t, lease.Context(), "lease was lost instead of renewed")

	competingLease, err := competingLocker.Acquire(context.Background(), "resource")
	require.Error(t, err)
	require.Nil(t, competingLease)
}

func TestLockerRenewsLeaseWithMinorityUnavailable(t *testing.T) {
	const leaseDuration = 300 * time.Millisecond
	cluster := newMemoryCluster(3)
	locker := newTestLocker(t, cluster, leaseDuration, oneAttemptBackOff)
	competingLocker := newTestLocker(t, cluster, leaseDuration, oneAttemptBackOff)

	lease := acquireLease(t, locker, "resource")
	unavailableObserved := cluster.nodes[2].makeUnavailable()
	originalValidityElapsed := time.NewTimer(leaseDuration + 100*time.Millisecond)
	defer originalValidityElapsed.Stop()
	defer func() {
		cluster.nodes[2].setAvailable(true)
		releaseLease(t, lease)
	}()

	requireSignal(t, unavailableObserved, time.Second, "unavailable node did not receive renewal")
	<-originalValidityElapsed.C
	requireContextActive(t, lease.Context(), "lease was lost after a minority node became unavailable")

	competingLease, err := competingLocker.Acquire(context.Background(), "resource")
	require.Error(t, err)
	require.Nil(t, competingLease)
}

func TestLockerLosesLeaseWhenNodesRestart(t *testing.T) {
	const leaseDuration = 300 * time.Millisecond
	cluster := newMemoryCluster(3)
	locker := newTestLocker(t, cluster, leaseDuration, oneAttemptBackOff)

	lease := acquireLease(t, locker, "resource")
	for _, node := range cluster.nodes {
		node.restart()
	}

	select {
	case <-lease.Context().Done():
		require.ErrorIs(t, context.Cause(lease.Context()), distlock.ErrLeaseLost)
	case <-time.After(time.Second):
		require.FailNow(t, "lease was not lost after its node state disappeared")
	}

	releaseLease(t, lease)
	require.ErrorIs(t, context.Cause(lease.Context()), distlock.ErrLeaseLost)
}
