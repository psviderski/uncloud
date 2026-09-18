package distlock

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMemoryStoreAcquire(t *testing.T) {
	store := newTestMemoryStore()
	ctx := context.Background()

	acquired, err := store.Acquire(ctx, "resource-a", []byte("owner-a"), time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)

	acquired, err = store.Acquire(ctx, "resource-a", []byte("owner-b"), time.Minute)
	require.NoError(t, err)
	require.False(t, acquired)

	acquired, err = store.Acquire(ctx, "resource-b", []byte("owner-b"), time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
}

func TestMemoryStoreExpirationAndStaleOwner(t *testing.T) {
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	store := newTestMemoryStore()
	store.now = func() time.Time { return now }
	ctx := context.Background()
	oldToken := []byte("old-owner")
	newToken := []byte("new-owner")

	acquired, err := store.Acquire(ctx, "resource", oldToken, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)

	// A lease is expired at its expiration time, not only after it.
	now = now.Add(time.Minute)
	acquired, err = store.Acquire(ctx, "resource", newToken, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)

	released, err := store.Release(ctx, "resource", oldToken)
	require.NoError(t, err)
	require.False(t, released)

	renewed, err := store.Renew(ctx, "resource", oldToken, time.Minute)
	require.NoError(t, err)
	require.False(t, renewed)

	released, err = store.Release(ctx, "resource", newToken)
	require.NoError(t, err)
	require.True(t, released)
}

func TestMemoryStoreRenew(t *testing.T) {
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	store := newTestMemoryStore()
	store.now = func() time.Time { return now }
	ctx := context.Background()
	token := []byte("owner")

	acquired, err := store.Acquire(ctx, "resource", token, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)

	now = now.Add(30 * time.Second)
	renewed, err := store.Renew(ctx, "resource", []byte("another-owner"), time.Minute)
	require.NoError(t, err)
	require.False(t, renewed)

	renewed, err = store.Renew(ctx, "resource", token, time.Minute)
	require.NoError(t, err)
	require.True(t, renewed)

	// The renewal extends the lease from the renewal time.
	now = now.Add(30 * time.Second)
	acquired, err = store.Acquire(ctx, "resource", []byte("another-owner"), time.Minute)
	require.NoError(t, err)
	require.False(t, acquired)

	now = now.Add(30 * time.Second)
	renewed, err = store.Renew(ctx, "resource", token, time.Minute)
	require.NoError(t, err)
	require.False(t, renewed)
}

func TestMemoryStoreRelease(t *testing.T) {
	store := newTestMemoryStore()
	ctx := context.Background()
	token := []byte("owner")

	released, err := store.Release(ctx, "resource", token)
	require.NoError(t, err)
	require.False(t, released)

	acquired, err := store.Acquire(ctx, "resource", token, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)

	released, err = store.Release(ctx, "resource", []byte("another-owner"))
	require.NoError(t, err)
	require.False(t, released)

	released, err = store.Release(ctx, "resource", token)
	require.NoError(t, err)
	require.True(t, released)

	released, err = store.Release(ctx, "resource", token)
	require.NoError(t, err)
	require.False(t, released)
}

func TestMemoryStoreCopiesToken(t *testing.T) {
	store := newTestMemoryStore()
	ctx := context.Background()
	token := []byte("owner")
	originalToken := append([]byte(nil), token...)

	acquired, err := store.Acquire(ctx, "resource", token, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)

	token[0] = 'x'
	released, err := store.Release(ctx, "resource", originalToken)
	require.NoError(t, err)
	require.True(t, released)
}

func TestMemoryStoreValidation(t *testing.T) {
	tests := []struct {
		name string
		run  func(*MemoryStore) error
	}{
		{
			name: "acquire with empty resource",
			run: func(store *MemoryStore) error {
				_, err := store.Acquire(context.Background(), "", []byte("owner"), time.Minute)
				return err
			},
		},
		{
			name: "acquire with empty token",
			run: func(store *MemoryStore) error {
				_, err := store.Acquire(context.Background(), "resource", nil, time.Minute)
				return err
			},
		},
		{
			name: "acquire with zero TTL",
			run: func(store *MemoryStore) error {
				_, err := store.Acquire(context.Background(), "resource", []byte("owner"), 0)
				return err
			},
		},
		{
			name: "renew with negative TTL",
			run: func(store *MemoryStore) error {
				_, err := store.Renew(context.Background(), "resource", []byte("owner"), -time.Second)
				return err
			},
		},
		{
			name: "release with empty token",
			run: func(store *MemoryStore) error {
				_, err := store.Release(context.Background(), "resource", nil)
				return err
			},
		},
		{
			name: "cancelled context",
			run: func(store *MemoryStore) error {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				_, err := store.Acquire(ctx, "resource", []byte("owner"), time.Minute)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newTestMemoryStore()
			require.Error(t, test.run(store))

			// Invalid operations must not create or replace a lease.
			acquired, err := store.Acquire(context.Background(), "resource", []byte("valid-owner"), time.Minute)
			require.NoError(t, err)
			require.True(t, acquired)
		})
	}
}

func TestMemoryStoreConcurrentAcquire(t *testing.T) {
	const attempts = 100

	store := newTestMemoryStore()
	ctx := context.Background()
	start := make(chan struct{})
	errCh := make(chan error, attempts)
	var acquired atomic.Int64
	var wg sync.WaitGroup

	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			ok, err := store.Acquire(ctx, "resource", []byte(fmt.Sprintf("owner-%d", i)), time.Minute)
			if err != nil {
				errCh <- err
				return
			}
			if ok {
				acquired.Add(1)
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, acquired.Load())
}

func newTestMemoryStore() *MemoryStore {
	store := NewMemoryStore()
	store.now = func() time.Time {
		return time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	}
	return store
}
