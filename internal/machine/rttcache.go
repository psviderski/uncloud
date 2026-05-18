package machine

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"github.com/psviderski/uncloud/internal/machine/cluster"
	"github.com/psviderski/uncloud/internal/machine/store"
)

const rttCacheRefreshInterval = 60 * time.Second

// RTTStats holds median and standard deviation of RTT samples to a machine.
type RTTStats struct {
	Median time.Duration
	StdDev time.Duration
}

// RTTCache caches round-trip time statistics to other machines in the cluster.
// It periodically refreshes from the Corrosion gossip system and maps management
// IPs to machine IDs for convenient lookup.
type RTTCache struct {
	machineID string
	cluster   *cluster.Cluster
	store     *store.Store

	mu      sync.RWMutex
	rttByID map[string]RTTStats
}

func newRTTCache(machineID string, cluster *cluster.Cluster, store *store.Store) *RTTCache {
	return &RTTCache{
		machineID: machineID,
		cluster:   cluster,
		store:     store,
		rttByID:   make(map[string]RTTStats),
	}
}

// Run starts the periodic refresh loop. It blocks until the context is cancelled.
func (c *RTTCache) Run(ctx context.Context) error {
	if err := c.refresh(ctx); err != nil {
		slog.Warn("Initial RTT cache refresh failed, will retry.", "err", err)
	}

	ticker := time.NewTicker(rttCacheRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.refresh(ctx); err != nil {
				slog.Warn("RTT cache refresh failed.", "err", err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// refresh fetches the latest RTT data from Corrosion and updates the cache.
func (c *RTTCache) refresh(ctx context.Context) error {
	rtts, err := c.cluster.MemberRTTs()
	if err != nil {
		return fmt.Errorf("get member rtts: %w", err)
	}

	machines, err := c.store.ListMachines(ctx)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}

	// Map management IP (without port) -> machine ID.
	ipToMachineID := make(map[netip.Addr]string, len(machines))
	for _, m := range machines {
		if m.Network == nil {
			continue
		}
		ip, err := m.Network.ManagementIp.ToAddr()
		if err != nil {
			continue
		}
		ipToMachineID[ip] = m.Id
	}

	newRTTByID := make(map[string]RTTStats, len(rtts)+1)
	// Always include self with RTT 0.
	newRTTByID[c.machineID] = RTTStats{Median: 0, StdDev: 0}

	for _, stats := range rtts {
		// Corrosion uses the management IP (with port) for gossip. Strip the port.
		if mid, ok := ipToMachineID[stats.Addr.Addr()]; ok {
			newRTTByID[mid] = RTTStats{
				Median: stats.Median,
				StdDev: stats.StdDev,
			}
		}
	}

	c.mu.Lock()
	c.rttByID = newRTTByID
	c.mu.Unlock()

	return nil
}

// ByMachineID returns the median RTT to the given machine. Returns (0, true) for
// the local machine, (duration, true) for known remote machines, and (0, false)
// for unknown machines.
func (c *RTTCache) ByMachineID(machineID string) (time.Duration, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats, ok := c.rttByID[machineID]
	if !ok {
		return 0, false
	}
	return stats.Median, true
}

// All returns a copy of all RTT statistics keyed by machine ID.
// The local machine is excluded from the result.
func (c *RTTCache) All() map[string]RTTStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]RTTStats, len(c.rttByID)-1)
	for mid, stats := range c.rttByID {
		if mid == c.machineID {
			continue
		}
		result[mid] = stats
	}
	return result
}
