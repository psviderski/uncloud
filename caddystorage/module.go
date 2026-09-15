// Package caddystorage provides Caddy storage backed by an Uncloud cluster.
package caddystorage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/certmagic"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/connector"
	"github.com/psviderski/uncloud/pkg/distlock"
)

const (
	// ModuleID is the Caddy module ID for Uncloud storage.
	ModuleID = "caddy.storage.uncloud"
	// DefaultSocketPath is the default path to the Uncloud API socket.
	DefaultSocketPath = "/run/uncloud/uncloud.sock"
	// DefaultLockTTL is the default duration of a distributed lock lease.
	DefaultLockTTL = 20 * time.Second
	// lockCleanupTimeout bounds how long an unloaded module waits for active lock operations when cleaning up.
	lockCleanupTimeout = 5 * time.Minute
)

//nolint:gochecknoinits // Caddy modules must register during package initialisation.
func init() {
	caddy.RegisterModule(new(Storage))
}

// Storage implements a Caddy storage backend that uses an Uncloud cluster to store assets such as TLS certificates.
type Storage struct {
	// Socket is the path to the Uncloud API socket.
	// Defaults to /run/uncloud/uncloud.sock when not set.
	Socket string `json:"socket,omitempty"`
	// LockTTL is the duration of a distributed lock after which it expires if not renewed. Locks renew automatically
	// until unlocked. If an instance crashes or cannot renew, expiry allows another instance to acquire the stale lock.
	// Longer durations tolerate longer interruptions but delay recovery after a crash. Normal unlocks release the lock
	// immediately.
	// Defaults to 20 seconds when not set.
	LockTTL caddy.Duration `json:"lock_ttl,omitempty"`

	client *client.Client
	locker *distlock.Locker
	log    *slog.Logger

	// ctx is the module context from Provision. Caddy cancels it when it unloads the module and calls Cleanup.
	ctx context.Context

	// locksMu protects locks.
	locksMu sync.Mutex
	// locks maps successfully acquired lock names to held leases.
	locks map[string]*distlock.Lease
	// lockOps counts Lock calls until they fail or their locks are released.
	lockOps atomic.Int64
}

// CaddyModule returns the Caddy module information.
func (*Storage) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  ModuleID,
		New: func() caddy.Module { return new(Storage) },
	}
}

// Provision connects the storage to the local Uncloud API and initialises the distributed locker.
func (s *Storage) Provision(ctx caddy.Context) error {
	s.ctx = ctx
	s.log = ctx.Slogger()

	if s.Socket == "" {
		s.Socket = DefaultSocketPath
	}
	if s.LockTTL == 0 {
		s.LockTTL = caddy.Duration(DefaultLockTTL)
	}
	if s.LockTTL < 0 {
		return errors.New("lock_ttl must be positive")
	}

	cli, err := client.New(ctx, connector.NewUnixConnector(s.Socket))
	if err != nil {
		return fmt.Errorf("connect to Uncloud API: %w", err)
	}

	locker, err := cli.NewLocker(distlock.Config{
		LeaseDuration: time.Duration(s.LockTTL),
	})
	if err != nil {
		_ = cli.Close()
		return fmt.Errorf("create distributed locker: %w", err)
	}

	s.client = cli
	s.locker = locker
	s.locks = make(map[string]*distlock.Lease)

	s.log.Info("module provisioned", "socket", s.Socket, "lock_ttl", time.Duration(s.LockTTL))
	return nil
}

// Cleanup keeps the Uncloud API connection open so acquired lock leases can renew until Caddy releases them. It closes
// the connection after all lock operations finish or the cleanup timeout expires.
func (s *Storage) Cleanup() error {
	if s.client == nil {
		// Provision failed before opening the storage.
		return nil
	}

	started := time.Now()
	s.log.Debug("cleaning up module", "locks", s.lockOps.Load())

	go func() {
		timer := time.NewTimer(lockCleanupTimeout)
		defer timer.Stop()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		timedOut := false
		for !timedOut && s.lockOps.Load() > 0 {
			select {
			case <-ticker.C:
			case <-timer.C:
				timedOut = true
			}
		}
		if remaining := s.lockOps.Load(); timedOut && remaining > 0 {
			s.log.Warn("timed out waiting for active locks to be unlocked",
				"locks", remaining, "timeout", lockCleanupTimeout)
		}

		if err := s.client.Close(); err != nil {
			s.log.Warn("failed to clean up module", "duration", time.Since(started), "error", err)
			return
		}
		s.log.Debug("module cleanup complete", "duration", time.Since(started))
	}()

	return nil
}

// CertMagicStorage returns the provisioned CertMagic storage implementation.
func (s *Storage) CertMagicStorage() (certmagic.Storage, error) {
	return s, nil
}

// UnmarshalCaddyfile configures Uncloud storage from the Caddyfile global storage block.
//
//	{
//	    storage uncloud {
//	        socket /run/uncloud/uncloud.sock
//	        lock_ttl 20s
//	    }
//	}
func (s *Storage) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	d.Next() // Skip the module name 'uncloud'.
	// Reject inline arguments. NextArg leaves an opening brace for NextBlock.
	if d.NextArg() {
		return d.ArgErr()
	}

	// Read the optional options block, skipping its surrounding braces.
	for d.NextBlock(0) {
		switch d.Val() {
		case "socket":
			// Require a socket path on the same line as 'socket' option.
			if !d.NextArg() {
				return d.ArgErr()
			}
			s.Socket = d.Val()
			// Reject extra arguments after the socket path.
			if d.NextArg() {
				return d.ArgErr()
			}
		case "lock_ttl":
			if !d.NextArg() {
				return d.ArgErr()
			}
			ttl, err := caddy.ParseDuration(d.Val())
			if err != nil {
				return d.Errf("invalid lock_ttl '%s': %v", d.Val(), err)
			}
			if ttl <= 0 {
				return d.Err("lock_ttl must be positive")
			}
			if d.NextArg() {
				return d.ArgErr()
			}
			s.LockTTL = caddy.Duration(ttl)
		default:
			return d.Errf("unknown uncloud storage option: '%s'", d.Val())
		}
	}

	return nil
}

var (
	_ caddy.Module           = (*Storage)(nil)
	_ caddy.Provisioner      = (*Storage)(nil)
	_ caddy.CleanerUpper     = (*Storage)(nil)
	_ caddy.StorageConverter = (*Storage)(nil)
	_ caddyfile.Unmarshaler  = (*Storage)(nil)
	_ certmagic.Storage      = (*Storage)(nil)
)
