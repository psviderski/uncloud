# Extract `rtt` Package and Unify RTT Lookup

## Context

`rttForMachine` in `caddyconfig/caddyfile.go:261` and `rttForResolved` in `dns/server.go:359` are nearly identical. Both check "is this local?" then look up RTT, falling back to `UnknownRTT`. The only difference is how they detect "local": caddyfile compares machine IDs, DNS checks if the IP is in the local subnet.

Since `ResolvedIP` already carries `MachineID` and the RTT cache already stores the local machine with RTT 0, the subnet check is redundant. Both functions can be replaced by a single `RTTFor(machineID)` method on the cache itself.

The cache currently lives in `internal/machine/rttcache.go` (package `machine`), but `dns/` and `caddyconfig/` are sub-packages that can't import `machine` (circular dependency). Solution: move the cache to a new `internal/machine/rtt/` package.

## Key Decisions

- **New package**: `internal/machine/rtt/`
- **`RTTFor` is a method on `*Cache`** (not a standalone function or function type)
- **`RTTFor` is nil-safe**: calling on a `nil` `*Cache` returns `UnknownRTT` (enables passing `nil` in tests)
- **`UnknownRTT` moves** from `constants` to `rtt` package
- **`RTTStats` moves** from `machine` to `rtt` package (renamed to `rtt.Stats`)
- **`localSubnet` removed** from `dns.Server` entirely (machine ID comparison replaces it)
- **Both `rttForMachine` and `rttForResolved` are deleted**; call sites use `cache.RTTFor(id)` directly
- **`NewCacheWithStats` constructor** added for test convenience (creates a pre-populated cache without needing mock providers)

## Dependency Graph (after)

```
rtt (new, no machine/* imports)
  ↑
  ├── dns
  ├── caddyconfig
  └── machine (imports rtt, passes *rtt.Cache to dns and caddyconfig)
```

---

## Milestone 1: Create the `rtt` Package

### Step 1: Create `internal/machine/rtt/rtt.go`

Create the new package with these contents:

```go
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
```

### Step 2: Create `internal/machine/rtt/rtt_test.go`

Move and adapt the tests from `internal/machine/rttcache_test.go`. Changes:
- Package: `package rtt`
- `RTTStats` → `Stats`
- `newRTTCache(...)` → `NewCache(...)`
- `TestRTTCache_*` → `TestCache_*`
- Mock types (`mockRTTProvider`, `mockMachineLister`, `newMockMachine`) stay the same but are unexported

Full content:

```go
package rtt

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/psviderski/uncloud/internal/corrosion"
	pb "github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testLocalMachineID = "local"

type mockRTTProvider struct {
	rtts []corrosion.MemberRTTStats
	err  error
}

func (m *mockRTTProvider) MemberRTTs() ([]corrosion.MemberRTTStats, error) {
	return m.rtts, m.err
}

type mockMachineLister struct {
	machines []*pb.MachineInfo
	err      error
}

func (m *mockMachineLister) ListMachines(ctx context.Context) ([]*pb.MachineInfo, error) {
	return m.machines, m.err
}

func newMockMachine(id, mgmtIP string) *pb.MachineInfo {
	return &pb.MachineInfo{
		Id: id,
		Network: &pb.NetworkConfig{
			ManagementIp: pb.NewIP(netip.MustParseAddr(mgmtIP)),
		},
	}
}

func TestCache_ByMachineID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		machineID string
		rtts      []corrosion.MemberRTTStats
		machines  []*pb.MachineInfo
		queryID   string
		wantRTT   time.Duration
		wantOK    bool
	}{
		{
			name:      "local machine returns zero RTT",
			machineID: testLocalMachineID,
			rtts:      nil,
			machines:  []*pb.MachineInfo{newMockMachine(testLocalMachineID, "10.0.0.1")},
			queryID:   testLocalMachineID,
			wantRTT:   0,
			wantOK:    true,
		},
		{
			name:      "known remote machine returns RTT",
			machineID: testLocalMachineID,
			rtts: []corrosion.MemberRTTStats{
				{Addr: netip.MustParseAddrPort("10.0.0.2:51000"), Median: 5 * time.Millisecond, StdDev: time.Millisecond},
			},
			machines: []*pb.MachineInfo{
				newMockMachine(testLocalMachineID, "10.0.0.1"),
				newMockMachine("remote", "10.0.0.2"),
			},
			queryID: "remote",
			wantRTT: 5 * time.Millisecond,
			wantOK:  true,
		},
		{
			name:      "unknown machine returns false",
			machineID: testLocalMachineID,
			rtts:      nil,
			machines:  []*pb.MachineInfo{newMockMachine(testLocalMachineID, "10.0.0.1")},
			queryID:   "unknown",
			wantRTT:   0,
			wantOK:    false,
		},
		{
			name:      "machine without network is skipped",
			machineID: testLocalMachineID,
			rtts: []corrosion.MemberRTTStats{
				{Addr: netip.MustParseAddrPort("10.0.0.2:51000"), Median: 5 * time.Millisecond, StdDev: time.Millisecond},
			},
			machines: []*pb.MachineInfo{
				newMockMachine(testLocalMachineID, "10.0.0.1"),
				{Id: "no-network"},
			},
			queryID: "no-network",
			wantRTT: 0,
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cache := NewCache(tt.machineID, &mockRTTProvider{rtts: tt.rtts}, &mockMachineLister{machines: tt.machines})
			err := cache.refresh(context.Background())
			require.NoError(t, err)

			rtt, ok := cache.ByMachineID(tt.queryID)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantRTT, rtt)
		})
	}
}

func TestCache_RTTFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cache   *Cache
		queryID string
		want    time.Duration
	}{
		{
			name:    "nil cache returns UnknownRTT",
			cache:   nil,
			queryID: "any",
			want:    UnknownRTT,
		},
		{
			name:    "local machine returns 0",
			cache:   NewCacheWithStats("local", map[string]Stats{"local": {Median: 0}}),
			queryID: "local",
			want:    0,
		},
		{
			name:    "known remote returns RTT",
			cache:   NewCacheWithStats("local", map[string]Stats{"remote": {Median: 5 * time.Millisecond}}),
			queryID: "remote",
			want:    5 * time.Millisecond,
		},
		{
			name:    "unknown machine returns UnknownRTT",
			cache:   NewCacheWithStats("local", map[string]Stats{"local": {Median: 0}}),
			queryID: "unknown",
			want:    UnknownRTT,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.cache.RTTFor(tt.queryID))
		})
	}
}

func TestCache_LivePeerRTTs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		machineID string
		rtts      []corrosion.MemberRTTStats
		machines  []*pb.MachineInfo
		wantRTTs  map[string]Stats
		wantErr   bool
	}{
		{
			name:      "excludes local machine",
			machineID: testLocalMachineID,
			rtts: []corrosion.MemberRTTStats{
				{Addr: netip.MustParseAddrPort("10.0.0.2:51000"), Median: 5 * time.Millisecond, StdDev: 1 * time.Millisecond},
				{Addr: netip.MustParseAddrPort("10.0.0.3:51000"), Median: 10 * time.Millisecond, StdDev: 2 * time.Millisecond},
			},
			machines: []*pb.MachineInfo{
				newMockMachine(testLocalMachineID, "10.0.0.1"),
				newMockMachine("peer1", "10.0.0.2"),
				newMockMachine("peer2", "10.0.0.3"),
			},
			wantRTTs: map[string]Stats{
				"peer1": {Median: 5 * time.Millisecond, StdDev: 1 * time.Millisecond},
				"peer2": {Median: 10 * time.Millisecond, StdDev: 2 * time.Millisecond},
			},
		},
		{
			name:      "empty cluster returns empty map",
			machineID: testLocalMachineID,
			rtts:      nil,
			machines:  []*pb.MachineInfo{newMockMachine(testLocalMachineID, "10.0.0.1")},
			wantRTTs:  map[string]Stats{},
		},
		{
			name:      "RTT from unknown IP is ignored",
			machineID: testLocalMachineID,
			rtts: []corrosion.MemberRTTStats{
				{Addr: netip.MustParseAddrPort("10.0.0.99:51000"), Median: 5 * time.Millisecond},
			},
			machines: []*pb.MachineInfo{newMockMachine(testLocalMachineID, "10.0.0.1")},
			wantRTTs: map[string]Stats{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cache := NewCache(tt.machineID, &mockRTTProvider{rtts: tt.rtts}, &mockMachineLister{machines: tt.machines})
			result, err := cache.LivePeerRTTs(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, tt.wantRTTs, result)
		})
	}
}

func TestCache_RefreshError(t *testing.T) {
	t.Parallel()

	cache := NewCache(testLocalMachineID, &mockRTTProvider{err: assert.AnError}, &mockMachineLister{machines: nil})
	err := cache.refresh(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get member rtts")

	cache = NewCache(testLocalMachineID, &mockRTTProvider{rtts: nil}, &mockMachineLister{err: assert.AnError})
	err = cache.refresh(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list machines")
}
```

### Step 3: Delete old files

- Delete `internal/machine/rttcache.go`
- Delete `internal/machine/rttcache_test.go`

---

## Milestone 2: Update Consumers

### Step 4: Remove `UnknownRTT` from `internal/machine/constants/constants.go`

Remove the `UnknownRTT` constant and the `"math"` import. The file should become:

```go
package constants

const (
	// MachineAPIPort is the port for the Machine API service on the management WireGuard network.
	MachineAPIPort = 51000
	// UnregistryPort is the port for the embedded container registry listening on the machine IP.
	UnregistryPort = 5000
)
```

Remove the `"time"` import as well since it was only used by `UnknownRTT`.

### Step 5: Update `internal/machine/dns/server.go`

1. **Imports**: Replace `"github.com/psviderski/uncloud/internal/machine/constants"` with `"github.com/psviderski/uncloud/internal/machine/rtt"`.

2. **`Server` struct** (line 51-63):
   - Remove `localSubnet netip.Prefix` field (line 53)
   - Change `rttByMachineID func(string) (time.Duration, bool)` (line 55) to `rttCache *rtt.Cache`

3. **`NewServer` signature** (line 68):
   - Remove `localSubnet netip.Prefix` parameter
   - Change `rttByMachineID func(string) (time.Duration, bool)` to `rttCache *rtt.Cache`
   - New signature: `func NewServer(listenAddr netip.Addr, resolver Resolver, upstreams []netip.AddrPort, rttCache *rtt.Cache) (*Server, error)`

4. **`NewServer` body** (line 100-108):
   - Remove `localSubnet: localSubnet,` (line 102)
   - Change `rttByMachineID: rttByMachineID,` (line 104) to `rttCache: rttCache,`

5. **`handleAQuery` method** (line 327):
   - Change `s.rttForResolved(a)` to `s.rttCache.RTTFor(a.MachineID)`
   - Change `s.rttForResolved(b)` to `s.rttCache.RTTFor(b.MachineID)`

6. **Delete `rttForResolved` method** (lines 356-369): Remove entirely.

### Step 6: Update `internal/machine/dns/server_test.go`

1. **Imports**: Replace `"github.com/psviderski/uncloud/internal/machine/constants"` with `"github.com/psviderski/uncloud/internal/machine/rtt"`.

2. **`TestServer_rttForResolved`** (lines 25-81): Rename to `TestCache_RTTFor_Integration` or similar. Rewrite to test via `rtt.Cache` directly instead of through the server:
   - Remove `localSubnet` from test setup
   - Replace `rttByMachineID func(string) (time.Duration, bool)` test field with `cache *rtt.Cache`
   - Build caches with `rtt.NewCacheWithStats(machineID, map[string]rtt.Stats{...})`
   - Call `cache.RTTFor(resolved.MachineID)` instead of `s.rttForResolved(resolved)`
   - Update "local subnet returns zero" test: use `rtt.NewCacheWithStats("local", map[string]rtt.Stats{"local": {}})` and query for `"local"` (instead of relying on subnet check with mismatched machine ID "remote")
   - Replace `constants.UnknownRTT` with `rtt.UnknownRTT`

3. **`TestServer_handleAQuery_NearestMode`** (lines 83-196):
   - Remove `localSubnet` from `Server` construction (lines 178-179)
   - Replace `rttByMachineID` field with `rttCache` field, built using `rtt.NewCacheWithStats`
   - In "local subnet IPs come first" test case (line 95): change `MachineID: "local"` for the `10.210.0.5` address (it already is `"local"`), and build cache with `rtt.NewCacheWithStats("local", map[string]rtt.Stats{"local": {}, "remote-1": {Median: 5*time.Millisecond}, "remote-2": {Median: 10*time.Millisecond}})`
   - For other test cases, build caches similarly from the inline function data
   - Remove `localSubnet` from all `Server` struct literals (lines 178, and any others)

4. **`TestServer_handleAQuery_RoundRobinMode`** (lines 198-233):
   - Remove `localSubnet` from `Server` struct literal (line 216)

5. **`TestServer_handleAQuery_NoResults`** (lines 235-250):
   - Remove `localSubnet` from `Server` struct literal (line 243)

### Step 7: Update `internal/machine/caddyconfig/caddyfile.go`

1. **Imports**: Replace `"github.com/psviderski/uncloud/internal/machine/constants"` with `"github.com/psviderski/uncloud/internal/machine/rtt"`. Keep `"time"` (still used on line 144 for `time.Now()` and `time.RFC3339`).

2. **`CaddyfileGenerator` struct** (lines 69-79):
   - Change `rttByMachineID func(string) (time.Duration, bool)` (line 76) to `rttCache *rtt.Cache`

3. **`NewCaddyfileGenerator` signature** (lines 86-88):
   - Change `rttByMachineID func(string) (time.Duration, bool)` parameter to `rttCache *rtt.Cache`
   - Update struct initialization (line 96): `rttByMachineID: rttByMachineID` → `rttCache: rttCache`

4. **`Generate` method** (line 128):
   - Change `g.rttForMachine(a.MachineID)` to `g.rttCache.RTTFor(a.MachineID)`
   - Change `g.rttForMachine(b.MachineID)` to `g.rttCache.RTTFor(b.MachineID)`

5. **Delete `rttForMachine` method** (lines 258-271): Remove entirely.

### Step 8: Update `internal/machine/caddyconfig/controller.go`

1. **Imports**: Add `"github.com/psviderski/uncloud/internal/machine/rtt"`. Remove `"time"` (only used by the old `rttByMachineID` function type on lines 35 and 61).

2. **`Controller` struct** (line 35):
   - Change `rttByMachineID func(string) (time.Duration, bool)` to `rttCache *rtt.Cache`

3. **`NewController` signature** (line 61):
   - Change `rttByMachineID func(string) (time.Duration, bool)` parameter to `rttCache *rtt.Cache`
   - Update struct initialization (line 78): `rttByMachineID: rttByMachineID` → `rttCache: rttCache`

4. **`Run` method** (line 93):
   - Change `c.rttByMachineID` to `c.rttCache` in the `NewCaddyfileGenerator` call

### Step 9: Update `internal/machine/caddyconfig/caddyfile_test.go`

Three call sites pass `nil` for the `rttByMachineID` parameter (lines 212, 856, 998):
```go
generator := NewCaddyfileGenerator("test-machine-id", "test-machine", nil, nil, nil)
```
These remain unchanged since `nil` is valid for `*rtt.Cache` and `RTTFor` is nil-safe.

No changes needed in this file.

### Step 10: Update `internal/machine/machine.go`

1. **Imports**: Add `"github.com/psviderski/uncloud/internal/machine/rtt"`. Keep `"github.com/psviderski/uncloud/internal/machine/constants"` (still used for `MachineAPIPort` on line 269 and `UnregistryPort` on line 507).

2. **`Machine` struct** (line 180):
   - Change `rttCache *RTTCache` to `rttCache *rtt.Cache`

3. **`Run` method** (line 467):
   - Change `newRTTCache(m.state.ID, m.cluster, m.store)` to `rtt.NewCache(m.state.ID, m.cluster, m.store)`

4. **`caddyconfig.NewController` call** (line 476):
   - Change `rttCache.ByMachineID` to `rttCache` (pass the cache instance, not the method)

5. **`dns.NewServer` call** (lines 483-489):
   - Remove `m.state.Network.Subnet` parameter
   - Change `rttCache.ByMachineID` to `rttCache`
   - New call:
     ```go
     dnsServer, err := dns.NewServer(
         m.IP(),
         dnsResolver,
         m.config.DNSUpstreams,
         rttCache,
     )
     ```

6. **`getMachineRTTs` method** (lines 1032-1050):
   - No changes needed. `m.rttCache.LivePeerRTTs(ctx)` returns `map[string]rtt.Stats` which has the same `.Median` and `.StdDev` fields as the old `machine.RTTStats`.

---

## Milestone 3: Verify

### Step 11: Build

```bash
go build ./...
```

Must succeed with no errors.

### Step 12: Run tests

```bash
go test ./internal/machine/rtt/... ./internal/machine/dns/... ./internal/machine/caddyconfig/... ./internal/machine/...
```

All tests must pass.

### Step 13: Lint

```bash
make lint
```

(or `golangci-lint run` if no Makefile target). Must pass clean.

---

## Open Questions

None. All design decisions have been resolved during planning.
