package distlock

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
)

const (
	DefaultLeaseDuration      = 10 * time.Second
	DefaultClockDriftFactor   = 0.01
	DefaultMaxNodeCallTimeout = 5 * time.Second
)

var (
	// ErrLeaseLost is the cancellation cause when automatic renewal can no longer maintain a lease.
	ErrLeaseLost = errors.New("distributed lease lost")
	// ErrLeaseReleased is the cancellation cause of an explicitly released lease.
	ErrLeaseReleased = errors.New("distributed lease released")
)

// Config configures a Locker.
type Config struct {
	// LeaseDuration is the TTL used for acquisitions and renewals. The default is DefaultLeaseDuration.
	LeaseDuration time.Duration
	// ClockDriftFactor is the fraction of LeaseDuration reserved for differences in clock rates between the Locker and
	// nodes. The default is DefaultClockDriftFactor.
	ClockDriftFactor float64
	// NodeCallTimeout sets the context timeout for an Acquire, Renew, or Release call to one node.
	// The default is the smaller of DefaultMaxNodeCallTimeout and one third of the lease duration.
	NodeCallTimeout time.Duration
	// NewBackOff creates independent retry policies for acquisitions and renewal cycles. The default is an exponential
	// backoff starting at 100ms and capped at 1s. A policy should not impose its own elapsed-time limit because the
	// acquisition context and current lease validity already bound retries.
	NewBackOff func() backoff.BackOff
}

func (c Config) clockDrift() time.Duration {
	return time.Duration(math.Ceil(float64(c.LeaseDuration) * c.ClockDriftFactor))
}

// Locker acquires automatically renewed distributed leases over a Cluster.
type Locker struct {
	config  Config
	cluster Cluster
}

// New creates a Locker over cluster.
func New(cluster Cluster, config Config) (*Locker, error) {
	if cluster == nil {
		return nil, fmt.Errorf("cluster is nil")
	}

	if config.LeaseDuration == 0 {
		config.LeaseDuration = DefaultLeaseDuration
	}
	if config.LeaseDuration < 0 {
		return nil, fmt.Errorf("lease duration must be positive")
	}
	if config.ClockDriftFactor == 0 {
		config.ClockDriftFactor = DefaultClockDriftFactor
	}
	if config.ClockDriftFactor <= 0 || config.ClockDriftFactor >= 1 {
		return nil, fmt.Errorf("clock drift factor must be greater than 0 and less than 1")
	}
	if config.NodeCallTimeout == 0 {
		config.NodeCallTimeout = min(DefaultMaxNodeCallTimeout, config.LeaseDuration/3)
	}
	if config.NodeCallTimeout < 0 {
		return nil, fmt.Errorf("node call timeout must be positive")
	}
	if config.NewBackOff == nil {
		config.NewBackOff = defaultBackOff
	}

	return &Locker{config: config, cluster: cluster}, nil
}

func defaultBackOff() backoff.BackOff {
	return backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(100*time.Millisecond),
		backoff.WithMaxInterval(time.Second),
		backoff.WithMaxElapsedTime(0),
	)
}

// Acquire waits until it acquires a lease for resource or ctx ends.
func (l *Locker) Acquire(ctx context.Context, resource string) (*Lease, error) {
	if resource == "" {
		return nil, fmt.Errorf("resource is empty")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	nodes, err := l.cluster.Nodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("get cluster nodes: %w", err)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("cluster has no nodes")
	}
	nodes = slices.Clone(nodes)

	boff := backoff.WithContext(l.config.NewBackOff(), ctx)
	var lease *Lease
	err = backoff.Retry(func() error {
		token, tokenErr := newOwnershipToken()
		if tokenErr != nil {
			return backoff.Permanent(fmt.Errorf("generate lease token: %w", tokenErr))
		}

		candidate := newLease(l, nodes, resource, token)
		if err := candidate.acquire(ctx); err != nil {
			return err
		}

		lease = candidate
		return nil
	}, boff)
	if err != nil {
		return nil, fmt.Errorf("acquire distributed lease for %q: %w", resource, err)
	}

	return lease, nil
}

// newOwnershipToken generates a unique 128-bit random ownership token for a lease.
func newOwnershipToken() ([]byte, error) {
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	return token, nil
}

// Lease is an automatically renewed distributed lease.
type Lease struct {
	config   Config
	nodes    []Node
	resource string
	token    []byte

	ctx    context.Context
	cancel context.CancelCauseFunc
	// done is closed when the renewal goroutine exits.
	done chan struct{}

	// operationMu prevents acquisition, renewal, and release operations for the lease from overlapping.
	operationMu sync.Mutex
	quorum      int
}

func newLease(locker *Locker, nodes []Node, resource string, token []byte) *Lease {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &Lease{
		config:   locker.config,
		nodes:    nodes,
		resource: resource,
		token:    token,
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
		quorum:   len(nodes)/2 + 1,
	}
}

// Context returns a context that is cancelled when the lease is lost or explicitly released.
func (l *Lease) Context() context.Context {
	return l.ctx
}

// Release stops automatic renewal and attempts to remove the lease from every node in its acquisition snapshot.
func (l *Lease) Release(ctx context.Context) error {
	l.cancel(ErrLeaseReleased)
	select {
	case <-l.done:
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := l.release(ctx); err != nil {
		return fmt.Errorf("release distributed lease for %q: %w", l.resource, err)
	}
	return nil
}

type nodeResult struct {
	success bool
	err     error
}

func collectNodeResults(results <-chan nodeResult) (successes int, err error) {
	var errs []error
	for result := range results {
		if result.err != nil {
			errs = append(errs, result.err)
		} else if result.success {
			successes++
		}
	}
	return successes, errors.Join(errs...)
}

func (l *Lease) executeNodes(ctx context.Context, fn func(context.Context, Node) (bool, error)) <-chan nodeResult {
	results := make(chan nodeResult, len(l.nodes))
	var wg sync.WaitGroup
	for _, node := range l.nodes {
		wg.Go(func() {
			callCtx, cancel := context.WithTimeout(ctx, l.config.NodeCallTimeout)
			defer cancel()

			success, err := fn(callCtx, node)
			results <- nodeResult{success: success, err: err}
		})
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}

// acquire makes one attempt to obtain the lease from a quorum of nodes and starts renewal on success.
func (l *Lease) acquire(ctx context.Context) error {
	startedAt := time.Now()
	validUntil := startedAt.Add(l.config.LeaseDuration - l.config.clockDrift())
	resultCh := make(chan error, 1)
	// Aggregate asynchronously so acquire can return at quorum while this goroutine drains the remaining results and
	// holds operationMu until every node call has finished.
	go func() {
		l.operationMu.Lock()
		defer l.operationMu.Unlock()

		// Use the lease context so cancelling the context passed to Acquire after it succeeds does not stop node calls
		// still pending after quorum. On failure, acquire cancels the lease context below. The validity deadline and
		// per-node NodeCallTimeout bound these calls.
		operationCtx, cancel := context.WithDeadline(l.ctx, validUntil)
		defer cancel()
		results := l.executeNodes(operationCtx, func(ctx context.Context, node Node) (bool, error) {
			return node.Acquire(ctx, l.resource, l.token, l.config.LeaseDuration)
		})

		successes := 0
		errs := make([]error, 0, len(l.nodes))
		reported := false
		for result := range results {
			if result.err != nil {
				errs = append(errs, result.err)
			} else if result.success {
				successes++
			}
			if !reported && successes >= l.quorum {
				resultCh <- nil
				reported = true
			}
		}
		if !reported {
			quorumErr := fmt.Errorf("lease acquired on %d of %d nodes, need at least %d",
				successes, len(l.nodes), l.quorum)
			resultCh <- errors.Join(quorumErr, errors.Join(errs...))
		}
	}()

	var acquireErr error
	select {
	case acquireErr = <-resultCh:
	case <-ctx.Done():
		acquireErr = ctx.Err()
	}

	if acquireErr == nil {
		if !time.Now().Before(validUntil) {
			acquireErr = fmt.Errorf("lease validity expired during acquisition")
		} else {
			go l.runRenew(validUntil)
			return nil
		}
	}

	// Cancel any node Acquire calls still in progress. release waits for them to finish before removing partial leases,
	// so no node can create this lease after cleanup has run.
	l.cancel(acquireErr)
	_ = l.release(context.WithoutCancel(ctx))
	return acquireErr
}

// release waits for any in-progress lease operation, removes the lease from every node, and returns any errors.
func (l *Lease) release(ctx context.Context) error {
	l.operationMu.Lock()
	defer l.operationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	results := l.executeNodes(ctx, func(ctx context.Context, node Node) (bool, error) {
		return node.Release(ctx, l.resource, l.token)
	})
	_, err := collectNodeResults(results)
	return err
}

// runRenew periodically renews the lease until it is released or lost.
func (l *Lease) runRenew(validUntil time.Time) {
	defer close(l.done)

	for {
		remaining := time.Until(validUntil)
		if remaining <= 0 {
			break
		}

		// Start renewal with two thirds of the current validity remaining
		// to leave time for slow node calls and retries.
		timer := time.NewTimer(remaining / 3)
		select {
		case <-l.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		renewedUntil, err := l.renew(validUntil)
		if err != nil {
			if l.ctx.Err() != nil {
				return
			}
			break
		}
		validUntil = renewedUntil
	}

	l.cancel(ErrLeaseLost)
	_ = l.release(context.Background())
}

// renew retries node renewals until a quorum succeeds, the configured backoff stops, or the current lease validity
// ends. It returns the new validity deadline after reaching quorum.
func (l *Lease) renew(currentValidUntil time.Time) (time.Time, error) {
	ctx, cancel := context.WithDeadline(l.ctx, currentValidUntil)
	defer cancel()

	var validUntil time.Time
	resultCh := make(chan error, 1)
	// Coordinate in the background so renew can report expiry even if a node call does not return after cancellation.
	// Keep operationMu held until every call finishes so cleanup cannot race a pending renewal.
	go func() {
		l.operationMu.Lock()
		defer l.operationMu.Unlock()

		if err := ctx.Err(); err != nil {
			resultCh <- err
			return
		}

		boff := backoff.WithContext(l.config.NewBackOff(), ctx)
		resultCh <- backoff.Retry(func() error {
			startedAt := time.Now()
			results := l.executeNodes(ctx, func(ctx context.Context, node Node) (bool, error) {
				return node.Renew(ctx, l.resource, l.token, l.config.LeaseDuration)
			})
			successes, nodeErr := collectNodeResults(results)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !time.Now().Before(currentValidUntil) {
				return backoff.Permanent(fmt.Errorf("renewal attempt took longer than its validity window"))
			}

			if successes >= l.quorum {
				validUntil = startedAt.Add(l.config.LeaseDuration - l.config.clockDrift())
				return nil
			}
			quorumErr := fmt.Errorf("lease renewed on %d of %d nodes, need at least %d",
				successes, len(l.nodes), l.quorum)
			return errors.Join(quorumErr, nodeErr)
		}, boff)
	}()

	select {
	case err := <-resultCh:
		return validUntil, err
	case <-ctx.Done():
		return time.Time{}, ctx.Err()
	}
}
