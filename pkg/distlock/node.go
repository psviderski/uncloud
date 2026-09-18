package distlock

import (
	"context"
	"time"
)

// Cluster provides point-in-time snapshots of the independent nodes that participate in distributed leases.
// Implementations must be safe for concurrent use.
type Cluster interface {
	// Nodes returns each node counted toward quorum exactly once, including nodes that are temporarily unavailable.
	// The Locker may keep and use the returned nodes throughout acquisition and until any acquired lease is released.
	Nodes(ctx context.Context) ([]Node, error)
}

// Node provides lease operations on a single cluster node.
//
// Each operation performs one attempt. A true result means the operation took effect. A false result with no error
// means the node responded but its lease state rejected the operation. If err is non-nil, the result is unknown and
// the boolean result must be ignored. Implementations must return promptly when ctx is cancelled, not modify token,
// and be safe for concurrent use.
type Node interface {
	// Acquire creates a lease when resource does not have an unexpired lease.
	Acquire(ctx context.Context, resource string, token []byte, ttl time.Duration) (bool, error)
	// Renew extends an unexpired lease when its ownership token matches.
	Renew(ctx context.Context, resource string, token []byte, ttl time.Duration) (bool, error)
	// Release removes an unexpired lease when its ownership token matches.
	Release(ctx context.Context, resource string, token []byte) (bool, error)
}
