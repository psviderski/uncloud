package rtt

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/netip"
	"sync"
	"time"

	"github.com/psviderski/uncloud/internal/corrosion"
	pb "github.com/psviderski/uncloud/internal/machine/api/pb"
)

const (
	rttCacheRefreshInterval = 60 * time.Second

	// UnknownRTT is a sentinel duration used to sort peers with unknown RTT last.
	UnknownRTT = time.Duration(math.MaxInt64)
)

// Stats holds median and standard deviation of RTT samples to a machine.
type Stats struct {
	Median time.Duration
	StdDev time.Duration
}

// rttProvider returns RTT statistics to cluster members.
type rttProvider interface {
	MemberRTTs() ([]corrosion.MemberRTTStats, error)
}

// machineLister lists machines in the cluster.
type machineLister interface {
	ListMachines(ctx context.Context) ([]*pb.MachineInfo, error)
}

// Cache caches round-trip time statistics to other machines in the cluster.
// It periodically refreshes from the Corrosion gossip system and maps management
// IPs to machine IDs for convenient lookup.
type Cache struct {
	machineID string
	cluster   rttProvider
	store     machineLister

	mu      sync.RWMutex
	rttByID map[string]Stats
}

// NewCache creates a new RTT cache that will refresh from the given cluster and store.
func NewCache(machineID string, cluster rttProvider, store machineLister) *Cache {
	return &Cache{
		machineID: machineID,
		cluster:   cluster,
		store:     store,
		rttByID:   make(map[string]Stats),
	}
}

// NewCacheWithStats creates a pre-populated RTT cache. Intended for tests.
func NewCacheWithStats(machineID string, rtts map[string]Stats) *Cache {
	return &Cache{
		machineID: machineID,
		rttByID:   rtts,
	}
}

// Run starts the periodic refresh loop. It blocks until the context is cancelled.
func (c *Cache) Run(ctx context.Context) error {
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
func (c *Cache) refresh(ctx context.Context) error {
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

	newRTTByID := make(map[string]Stats, len(rtts)+1)
	// Always include self with RTT 0.
	newRTTByID[c.machineID] = Stats{Median: 0, StdDev: 0}

	for _, stats := range rtts {
		// Corrosion uses the management IP (with port) for gossip. Strip the port.
		if mid, ok := ipToMachineID[stats.Addr.Addr()]; ok {
			newRTTByID[mid] = Stats{
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
func (c *Cache) ByMachineID(machineID string) (time.Duration, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats, ok := c.rttByID[machineID]
	if !ok {
		return 0, false
	}
	return stats.Median, true
}

// RTTFor returns the RTT for a machine: 0 for the local machine (already stored
// with RTT 0), the median RTT for known remote machines, and UnknownRTT for
// unknown machines (sorted last). Nil-safe: returns UnknownRTT on a nil Cache.
func (c *Cache) RTTFor(machineID string) time.Duration {
	if c == nil {
		return UnknownRTT
	}
	if rtt, ok := c.ByMachineID(machineID); ok {
		return rtt
	}
	return UnknownRTT
}

// LivePeerRTTs returns a fresh snapshot of RTT statistics for all peer machines
// (excluding the local machine). It performs a live refresh from Corrosion before
// returning, so the data is recent.
func (c *Cache) LivePeerRTTs(ctx context.Context) (map[string]Stats, error) {
	if err := c.refresh(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]Stats, len(c.rttByID)-1)
	for mid, stats := range c.rttByID {
		if mid == c.machineID {
			continue
		}
		result[mid] = stats
	}
	return result, nil
}
