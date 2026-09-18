package distlock

import (
	"context"
	"time"
)

// Store holds node-local lease state and provides atomic operations over it.
type Store interface {
	// Acquire creates a lease when the resource does not have an unexpired lease.
	Acquire(ctx context.Context, resource string, token []byte, ttl time.Duration) (bool, error)
	// Renew extends an unexpired lease when its ownership token matches.
	Renew(ctx context.Context, resource string, token []byte, ttl time.Duration) (bool, error)
	// Release removes a lease when its ownership token matches.
	Release(ctx context.Context, resource string, token []byte) (bool, error)
}
