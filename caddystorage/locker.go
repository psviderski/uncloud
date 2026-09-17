package caddystorage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/distlock"
)

const (
	lockPrefix              = "caddy_storage:"
	storeReplicationTimeout = 10 * time.Second
)

// Lock acquires an automatically renewed distributed lock and waits for the local store to catch up with versions
// observed on responding machines. Writers using the same lock can then read locally. Unavailable machines may have
// writes that this wait does not cover, and reads outside a lock remain eventually consistent.
func (s *Storage) Lock(ctx context.Context, name string) (err error) {
	if name == "" {
		return errors.New("lock name is empty")
	}

	// Count the call first. If Cleanup has already observed zero, Caddy has cancelled s.ctx and the check below rejects
	// this call before it uses the client.
	s.lockOps.Add(1)
	// A failed Lock owns its lifecycle through any lease rollback. A successful Lock transfers that responsibility to
	// Unlock, which keeps the client open until its release attempt finishes.
	defer func() {
		if err != nil {
			s.lockOps.Add(-1)
		}
	}()
	if s.ctx.Err() != nil {
		return errors.New("storage is closed")
	}

	// Stop acquisition retries when Caddy unloads the module (cancels s.ctx), even if the caller's context
	// is still active.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	stopOnCleanup := context.AfterFunc(s.ctx, func() {
		cancel(context.Cause(s.ctx))
	})
	defer stopOnCleanup()

	log := s.log.With("lock", name)
	started := time.Now()
	log.Debug("acquiring lock", "ttl", time.Duration(s.LockTTL))
	lease, err := s.locker.Acquire(ctx, lockPrefix+name)
	if err != nil {
		log.Debug("failed to acquire lock", "duration", time.Since(started), "error", err)
		return fmt.Errorf("acquire lock '%s': %w", name, err)
	}
	log.Debug("lock lease acquired", "duration", time.Since(started))
	// Release the lease if the lock acquisition fails after this point.
	// Unlock will take care of releasing the lease on success.
	defer func() {
		if err == nil {
			return
		}
		if releaseErr := s.releaseLock(ctx, name, lease, "cancelled acquisition"); releaseErr != nil {
			err = errors.Join(err, releaseErr)
		}
	}()

	// Catch up with the latest store versions observed on responding machines to increase the chance of reading
	// the latest writes on them locally.
	version, machines, err := s.clusterStoreVersion(ctx, log)
	if err != nil {
		return err
	}
	waitStarted := time.Now()
	log.Debug("waiting for local store replication", "machine_names", machines, "store_version", version)
	waitCtx, cancelWait := context.WithTimeout(ctx, storeReplicationTimeout)
	err = s.client.WaitForStoreVersion(waitCtx, version)
	cancelWait()
	if err != nil {
		return fmt.Errorf("wait for local store replication: %w", err)
	}
	log.Debug("local store replication complete", "duration", time.Since(waitStarted))

	s.locksMu.Lock()
	defer s.locksMu.Unlock()
	if s.ctx.Err() != nil {
		return errors.New("storage is closed")
	}
	if lost := context.Cause(lease.Context()); lost != nil {
		return lost
	}
	if _, exists := s.locks[name]; exists {
		return errors.New("lock is already tracked by this storage instance")
	}
	s.locks[name] = lease

	log.Debug("lock acquired", "duration", time.Since(started))
	return nil
}

// clusterStoreVersion returns the per-actor maximum store versions from responding machines and their names.
func (s *Storage) clusterStoreVersion(ctx context.Context, log *slog.Logger) (map[string]uint64, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, distlock.DefaultMaxNodeCallTimeout)
	defer cancel()
	resp, err := s.client.MachineClient.InspectMachine(client.ProxyMachinesContext(ctx, nil), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect machines for store versions: %w", err)
	}

	maxVersion := make(map[string]uint64)
	machines := make([]string, 0, len(resp.Machines))
	for _, m := range resp.Machines {
		if m.Metadata.Error != "" {
			log.Warn("skipping machine when collecting store versions",
				"id", m.Metadata.MachineId, "name", m.Metadata.MachineName, "error", m.Metadata.Error)
			continue
		}
		machines = append(machines, m.Metadata.MachineName)
		for actor, v := range m.StoreVersion {
			maxVersion[actor] = max(maxVersion[actor], v)
		}
	}
	slices.Sort(machines)
	return maxVersion, machines, nil
}

// Unlock releases a previously acquired distributed lock.
func (s *Storage) Unlock(ctx context.Context, name string) error {
	s.locksMu.Lock()
	lease, exists := s.locks[name]
	if exists {
		delete(s.locks, name)
	}
	s.locksMu.Unlock()
	if !exists {
		return fmt.Errorf("lock '%s' is not held by this storage instance", name)
	}
	defer s.lockOps.Add(-1)

	// Release stops renewal even on error. Any nodes that cannot be reached will let the lease expire.
	if err := s.releaseLock(ctx, name, lease, "unlock"); err != nil {
		return fmt.Errorf("release lock '%s': %w", name, err)
	}

	return nil
}

// releaseLock releases the lease with a context that ignores cancellation and logs the release attempt.
func (s *Storage) releaseLock(ctx context.Context, name string, lease *distlock.Lease, reason string) error {
	// Unlock and rollback must attempt node cleanup even if the caller or module has already been cancelled.
	ctx = context.WithoutCancel(ctx)

	log := s.log.With("lock", name, "reason", reason)
	started := time.Now()
	log.Debug("releasing lock")
	if err := lease.Release(ctx); err != nil {
		log.Debug("failed to release lock", "duration", time.Since(started), "error", err)
		return err
	}
	log.Debug("lock released", "duration", time.Since(started))
	return nil
}
