package distlock

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"
)

type memoryLease struct {
	token     []byte
	expiresAt time.Time
}

// MemoryStore stores leases in memory. Its contents are lost when the process exits.
type MemoryStore struct {
	mu     sync.Mutex
	leases map[string]memoryLease
	now    func() time.Time
}

// NewMemoryStore creates an empty in-memory lease store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		leases: make(map[string]memoryLease),
		now:    time.Now,
	}
}

// Acquire creates a lease when the resource does not have an unexpired lease.
func (s *MemoryStore) Acquire(
	ctx context.Context, resource string, token []byte, ttl time.Duration,
) (bool, error) {
	if err := validateStoreInput(ctx, resource, token, ttl); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	now := s.now()

	if lease, ok := s.leases[resource]; ok && now.Before(lease.expiresAt) {
		return false, nil
	}

	s.leases[resource] = memoryLease{
		token:     bytes.Clone(token),
		expiresAt: now.Add(ttl),
	}
	return true, nil
}

// Renew extends an unexpired lease when its ownership token matches.
func (s *MemoryStore) Renew(
	ctx context.Context, resource string, token []byte, ttl time.Duration,
) (bool, error) {
	if err := validateStoreInput(ctx, resource, token, ttl); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	now := s.now()

	lease, ok := s.leases[resource]
	if !ok {
		return false, nil
	}
	if !now.Before(lease.expiresAt) {
		delete(s.leases, resource)
		return false, nil
	}
	if !bytes.Equal(lease.token, token) {
		return false, nil
	}

	lease.expiresAt = now.Add(ttl)
	s.leases[resource] = lease
	return true, nil
}

// Release removes an unexpired lease when its ownership token matches.
func (s *MemoryStore) Release(ctx context.Context, resource string, token []byte) (bool, error) {
	if err := validateStoreResourceToken(ctx, resource, token); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	now := s.now()

	lease, ok := s.leases[resource]
	if !ok {
		return false, nil
	}
	if !now.Before(lease.expiresAt) {
		delete(s.leases, resource)
		return false, nil
	}
	if !bytes.Equal(lease.token, token) {
		return false, nil
	}

	delete(s.leases, resource)
	return true, nil
}

func validateStoreInput(ctx context.Context, resource string, token []byte, ttl time.Duration) error {
	if err := validateStoreResourceToken(ctx, resource, token); err != nil {
		return err
	}
	if ttl <= 0 {
		return fmt.Errorf("TTL must be positive")
	}
	return nil
}

func validateStoreResourceToken(ctx context.Context, resource string, token []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if resource == "" {
		return fmt.Errorf("resource is empty")
	}
	if len(token) == 0 {
		return fmt.Errorf("token is empty")
	}
	return nil
}
