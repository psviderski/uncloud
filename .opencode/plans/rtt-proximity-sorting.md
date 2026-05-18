# RTT Proximity Sorting Implementation Plan

This plan implements RTT-based proximity sorting for DNS and Caddy routing, replacing the binary local/remote distinction with continuous RTT-based ordering.

## Key Design Decisions

1. **New file `internal/machine/rttcache.go`** in `package machine` — defines `RTTCache` type with periodic refresh, management-IP-to-machine mapping, `ByMachineID`, `All` methods. Takes `*cluster.Cluster` and `*store.Store` as constructor params since it's in the same package as `Machine`.

2. **DNS `Resolver` interface** in `internal/machine/dns/server.go` changes from `Resolve(serviceName string) []netip.Addr` to `Resolve(serviceName string) []ResolvedIP` where `ResolvedIP struct { Addr netip.Addr; MachineID string }` is a new exported type in the `dns` package.

3. **DNS resolver** (`internal/machine/dns/resolver.go`) stores `[]ResolvedIP` instead of `[]netip.Addr` in its `serviceIPs` map. The `MachineID` comes from `record.MachineID` which is already available in `updateServiceIPs`.

4. **DNS server** (`internal/machine/dns/server.go`) gains a `rttByMachineID func(string) (time.Duration, bool)` field. The `nearest` mode sort changes from binary local/not-local to `cmp.Compare(s.rttForResolved(a), s.rttForResolved(b))` using `slices.SortStableFunc`.

5. **Caddy generator** (`internal/machine/caddyconfig/caddyfile.go`) gains `rttByMachineID func(string) (time.Duration, bool)` field. `localMachineRank` is replaced by `rttForMachine` which returns `time.Duration` (0 for local, RTT for known remote, `time.Duration(math.MaxInt64)` for unknown). The sort changes from `g.localMachineRank(a)-g.localMachineRank(b)` to `cmp.Compare(g.rttForMachine(...), g.rttForMachine(...))`.

6. **Caddy controller** (`internal/machine/caddyconfig/controller.go`) threads `rttByMachineID` through to `CaddyfileGenerator`.

7. **Machine** (`internal/machine/machine.go`) creates `RTTCache`, starts it in errGroup, passes `rttCache.ByMachineID` to DNS server and Caddy controller constructors. `getMachineRTTs` is simplified to delegate to `rttCache.All()`.

8. **Both `rttByMachineID` functions are nil-able** — if nil, the fallback in `rttForResolved`/`rttForMachine` treats all remote machines as unknown (`math.MaxInt64`). This preserves backward-compatible behavior and allows tests to pass nil.

9. **RTT cache stores both `Median` and `StdDev`** since they come from the same Corrosion fetch. `ByMachineID` returns just the median for ordering. `All()` returns full stats for the gRPC display endpoint.

10. **`maps` import is removed from `dns/resolver.go`** since `maps.EqualFunc` is replaced by a custom `resolvedIPsEqual` function.

---

## Step-by-step instructions

> ✅ **Step 1: Create `internal/machine/rttcache.go`**
> - Create a new file `internal/machine/rttcache.go` in `package machine`. The file contains the `RTTCache` type with periodic refresh, IP-to-machine-ID mapping, and `ByMachineID`/`All` methods.
> - Expected outcome: The file compiles as part of `package machine` and provides the RTT caching infrastructure.

Create a new file `internal/machine/rttcache.go` with the following content:

```go
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
```

Important details:
- The `ipToMachineID` map uses `netip.Addr` keys (not strings), matching the pattern in the existing `getMachineRTTs` method.
- `MemberRTTs()` returns `[]corrosion.MemberRTTStats` where `stats.Addr` is `netip.AddrPort`. `stats.Addr.Addr()` strips the port to get `netip.Addr`.
- `m.Network.ManagementIp.ToAddr()` returns `(netip.Addr, error)`. Errors are skipped with `continue` (matching existing `getMachineRTTs` behavior which uses `_, _` to ignore errors).
- The `refresh` method does NOT fail on individual machine parsing errors — it skips them.
- The initial `refresh` failure in `Run` is logged as a warning and does NOT cause `Run` to return an error. The ticker will retry.

---

> ✅ **Step 2: Modify `internal/machine/dns/server.go`**
> - Add `ResolvedIP` type, update `Resolver` interface, add `rttByMachineID` field to `Server`, update `NewServer` signature, and replace `handleAQuery` with RTT-based sorting.
> - Expected outcome: DNS server supports `nearest` mode sorting by RTT proximity.

#### 2a: Add imports
Add `"cmp"` and `"math"` to the import block. Keep all existing imports.

Current imports (lines 3-18):
```go
import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)
```

New imports:
```go
import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)
```

#### 2b: Add `ResolvedIP` type and update `Resolver` interface (lines 33-38)
Replace the current `Resolver` interface with:

```go
// ResolvedIP pairs an IP address with the machine ID where the container is running.
type ResolvedIP struct {
	Addr      netip.Addr
	MachineID string
}

// Resolver is an interface for resolving service names to IP addresses with machine metadata.
type Resolver interface {
	// Resolve returns a list of resolved IPs for the service containers.
	// An empty list is returned if no service is found.
	Resolve(serviceName string) []ResolvedIP
}
```

#### 2c: Add `rttByMachineID` field to `Server` struct (lines 42-53)
Add the `rttByMachineID` field after `resolver`:

```go
type Server struct {
	listenAddr      netip.Addr
	localSubnet     netip.Prefix
	resolver        Resolver
	rttByMachineID  func(string) (time.Duration, bool)
	upstreamServers []netip.AddrPort

	udpServer        *dns.Server
	tcpServer        *dns.Server
	inProgressReqs   sync.WaitGroup
	forwardSemaphore chan struct{}
	log              *slog.Logger
}
```

#### 2d: Update `NewServer` signature and body (line 58)
Change signature from:
```go
func NewServer(listenAddr netip.Addr, localSubnet netip.Prefix, resolver Resolver, upstreams []netip.AddrPort) (*Server, error) {
```
To:
```go
func NewServer(listenAddr netip.Addr, localSubnet netip.Prefix, resolver Resolver, upstreams []netip.AddrPort, rttByMachineID func(string) (time.Duration, bool)) (*Server, error) {
```

In the return statement (lines 90-97), add `rttByMachineID:` to the struct literal:
```go
	return &Server{
		listenAddr:       listenAddr,
		localSubnet:      localSubnet,
		resolver:         resolver,
		rttByMachineID:   rttByMachineID,
		upstreamServers:  upstreams,
		forwardSemaphore: make(chan struct{}, maxConcurrentForwards),
		log:              slog.With("component", "dns-server"),
	}, nil
```

#### 2e: Replace `handleAQuery` method (lines 292-342)
Replace the entire `handleAQuery` method with:

```go
// handleAQuery processes an A query for the internal domain and returns A records for the requested name.
// The internal domain suffix is already stripped from the name. An empty list is returned if no records are found.
func (s *Server) handleAQuery(name string) []dns.RR {
	serviceName, mode := extractModeFromDomain(trimInternalDomain(name))
	resolved := s.resolver.Resolve(serviceName)
	if len(resolved) == 0 {
		s.log.Debug("Failed to resolve service name.", "service", serviceName)
		return nil
	}
	s.log.Debug("Resolved service name.", "service", serviceName, "resolved", resolved)

	if len(resolved) > 1 {
		// Shuffle the resolved IPs to approximate round-robin.
		// We want to do this as a baseline for "nearest" mode, as well.
		rand.Shuffle(len(resolved), func(i, j int) {
			resolved[i], resolved[j] = resolved[j], resolved[i]
		})

		// Default (mode == "") currently behaves the same as round-robin,
		// and nothing additional to do for round-robin (mode == "rr").

		if mode == "nearest" {
			// Sort by RTT using proximity data. Local machine containers get RTT 0.
			slices.SortStableFunc(resolved, func(a, b ResolvedIP) int {
				return cmp.Compare(s.rttForResolved(a), s.rttForResolved(b))
			})
		}
	}

	// Extract IPs from resolved IPs for DNS response.
	ips := make([]netip.Addr, len(resolved))
	for i, r := range resolved {
		ips[i] = r.Addr
	}

	// Create A records for each IP.
	records := make([]dns.RR, 0, len(ips))
	for _, ip := range ips {
		records = append(records, &dns.A{
			Hdr: dns.RR_Header{
				Name:   name,
				Rtype:  dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    0,
			},
			A: net.ParseIP(ip.String()),
		})
	}
	return records
}

// rttForResolved returns the RTT for a resolved IP. Returns 0 if the IP is on the
// local subnet (same machine), looks up RTT via rttByMachineID for remote machines,
// and returns math.MaxInt64 for unknown machines (sorted last).
func (s *Server) rttForResolved(r ResolvedIP) time.Duration {
	if s.localSubnet.Contains(r.Addr) {
		return 0
	}
	if s.rttByMachineID != nil {
		if rtt, ok := s.rttByMachineID(r.MachineID); ok {
			return rtt
		}
	}
	return time.Duration(math.MaxInt64)
}
```

**Important notes for this step:**
- The `handleAQuery` method changes from using `[]netip.Addr` to `[]ResolvedIP` throughout.
- The shuffle now shuffles `resolved` (type `[]ResolvedIP`) instead of `ips` (`[]netip.Addr`).
- `slices.SortFunc` is replaced with `slices.SortStableFunc` to preserve the shuffle order for equal-RTT entries.
- `cmp.Compare` replaces the manual comparison. The import `"cmp"` must be added.
- The `rttForResolved` method includes a nil check on `s.rttByMachineID` to support tests that pass nil.
- The "Create A records" loop is unchanged — it extracts IPs from `resolved` first, then creates records from the plain IPs.

---

> ✅ **Step 3: Modify `internal/machine/dns/resolver.go`**
> - Update `serviceIPs` map type to `[]ResolvedIP`, update `Resolve` return type, replace `maps.EqualFunc` with custom `resolvedIPsEqual`, remove `"maps"` import.
> - Expected outcome: DNS resolver carries machine ID alongside IP addresses.

#### 3a: Update `serviceIPs` field type (lines 18-27)
Change `ClusterResolver` struct:

From:
```go
type ClusterResolver struct {
	store *store.Store
	// serviceIPs maps service names to container IPs.
	serviceIPs map[string][]netip.Addr
	// mu protects the serviceIPs map.
	mu sync.RWMutex
	// lastUpdate tracks when records were last updated.
	lastUpdate time.Time
	log        *slog.Logger
}
```

To:
```go
type ClusterResolver struct {
	store *store.Store
	// serviceIPs maps service names to resolved container IPs with machine metadata.
	serviceIPs map[string][]ResolvedIP
	// mu protects the serviceIPs map.
	mu sync.RWMutex
	// lastUpdate tracks when records were last updated.
	lastUpdate time.Time
	log        *slog.Logger
}
```

#### 3b: Update `NewClusterResolver` (lines 30-36)
Change the initialization of `serviceIPs`:

From:
```go
func NewClusterResolver(store *store.Store) *ClusterResolver {
	return &ClusterResolver{
		store:      store,
		serviceIPs: make(map[string][]netip.Addr),
		log:        slog.With("component", "dns-resolver"),
	}
}
```

To:
```go
func NewClusterResolver(store *store.Store) *ClusterResolver {
	return &ClusterResolver{
		store:      store,
		serviceIPs: make(map[string][]ResolvedIP),
		log:        slog.With("component", "dns-resolver"),
	}
}
```

#### 3c: Replace `updateServiceIPs` method (lines 72-122)
Replace the entire `updateServiceIPs` method with:

```go
// updateServiceIPs processes container records and updates the serviceIPs map.
func (r *ClusterResolver) updateServiceIPs(containers []store.ContainerRecord) {
	newServiceIPs := make(map[string][]ResolvedIP, len(r.serviceIPs))

	containersCount := 0
	for _, record := range containers {
		if record.Container.IsHook() {
			continue
		}
		if !record.Container.Healthy() {
			continue
		}

		ip := record.Container.UncloudNetworkIP()
		if !ip.IsValid() {
			// Container is not connected to the uncloud Docker network (could be host network).
			continue
		}

		ctr := record.Container
		if ctr.ServiceID() == "" || ctr.ServiceName() == "" {
			// Container is not part of a service, skip it.
			continue
		}

		resolved := ResolvedIP{Addr: ip, MachineID: record.MachineID}
		newServiceIPs[ctr.ServiceName()] = append(newServiceIPs[ctr.ServiceName()], resolved)
		// Also add the service ID as a valid lookup.
		newServiceIPs[ctr.ServiceID()] = append(newServiceIPs[ctr.ServiceID()], resolved)

		// Add <machine-id>.m.<service-name> as a lookup
		serviceNameWithMachineID := record.MachineID + ".m." + ctr.ServiceName()
		newServiceIPs[serviceNameWithMachineID] = append(newServiceIPs[serviceNameWithMachineID], resolved)

		containersCount++
	}

	// Sort each service's resolved IPs by address for deterministic order and comparison.
	for _, ips := range newServiceIPs {
		slices.SortFunc(ips, func(a, b ResolvedIP) int { return a.Addr.Compare(b.Addr) })
	}
	// Skip the swap when the services or their container IPs haven't changed.
	if resolvedIPsEqual(r.serviceIPs, newServiceIPs) {
		return
	}

	// Update the serviceIPs map atomically.
	r.mu.Lock()
	r.serviceIPs = newServiceIPs
	r.mu.Unlock()

	r.log.Info("DNS records updated.", "services", len(newServiceIPs)/3, "containers", containersCount)
}

// resolvedIPsEqual returns true if two maps of resolved IP slices are equal.
func resolvedIPsEqual(a, b map[string][]ResolvedIP) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok || len(v) != len(bv) {
			return false
		}
		for i := range v {
			if v[i] != bv[i] {
				return false
			}
		}
	}
	return true
}
```

**Note:** The `maps.EqualFunc(r.serviceIPs, newServiceIPs, slices.Equal[[]netip.Addr])` is replaced by the custom `resolvedIPsEqual` function. The import `"maps"` should be removed from the imports.

#### 3d: Update `Resolve` return type (lines 125-139)
Change:

From:
```go
// Resolve returns IP addresses of the service containers.
func (r *ClusterResolver) Resolve(serviceName string) []netip.Addr {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ips, ok := r.serviceIPs[serviceName]
	if !ok || len(ips) == 0 {
		return nil
	}

	// Return a copy of the IPs slice to prevent modification of the original.
	ipsCopy := make([]netip.Addr, len(ips))
	copy(ipsCopy, ips)

	return ipsCopy
}
```

To:
```go
// Resolve returns resolved IPs of the service containers.
func (r *ClusterResolver) Resolve(serviceName string) []ResolvedIP {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resolved, ok := r.serviceIPs[serviceName]
	if !ok || len(resolved) == 0 {
		return nil
	}

	// Return a copy of the slice to prevent modification of the original.
	resolvedCopy := make([]ResolvedIP, len(resolved))
	copy(resolvedCopy, resolved)

	return resolvedCopy
}
```

#### 3e: Update imports
Remove `"maps"` from the import block (it's no longer used — `resolvedIPsEqual` replaces `maps.EqualFunc`). Keep all other imports including `"slices"` (still used for `SortFunc`).

The final import block should be:
```go
import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"sync"
	"time"

	"github.com/psviderski/uncloud/internal/machine/store"
)
```

---

> ✅ **Step 4: Modify `internal/machine/caddyconfig/caddyfile.go`**
> - Add `"math"` import, add `rttByMachineID` field to `CaddyfileGenerator`, update `NewCaddyfileGenerator` signature, replace `localMachineRank` with `rttForMachine`, update sort comparison.
> - Expected outcome: Caddy generator sorts containers by RTT proximity instead of local/remote binary.

#### 4a: Add `"math"` to imports (lines 3-19)
Add `"math"` to the import block. The current imports are:
```go
import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
)
```

New imports:
```go
import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"net"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
)
```

#### 4b: Add `rttByMachineID` field to `CaddyfileGenerator` struct (lines 68-75)
Change from:
```go
type CaddyfileGenerator struct {
	// machineID is the unique identifier of the machine where the controller is running.
	machineID string
	// machineName is the human-friendly name of the machine.
	machineName string
	validator   CaddyfileValidator
	log         *slog.Logger
}
```

To:
```go
type CaddyfileGenerator struct {
	// machineID is the unique identifier of the machine where the controller is running.
	machineID string
	// machineName is the human-friendly name of the machine.
	machineName string
	// rttByMachineID returns the median RTT to a given machine. Returns nil for no RTT data.
	// If nil, all remote machines are treated as having unknown RTT (sorted last).
	rttByMachineID func(string) (time.Duration, bool)
	validator   CaddyfileValidator
	log         *slog.Logger
}
```

#### 4c: Update `NewCaddyfileGenerator` (lines 82-94)
Change from:
```go
func NewCaddyfileGenerator(
	machineID, machineName string, validator CaddyfileValidator, log *slog.Logger,
) *CaddyfileGenerator {
```

To:
```go
func NewCaddyfileGenerator(
	machineID, machineName string, rttByMachineID func(string) (time.Duration, bool),
	validator CaddyfileValidator, log *slog.Logger,
) *CaddyfileGenerator {
```

And add `rttByMachineID:` to the return struct:
```go
	return &CaddyfileGenerator{
		machineID:      machineID,
		machineName:    machineName,
		rttByMachineID: rttByMachineID,
		validator:      validator,
		log:            log,
	}
```

#### 4d: Update the sort in `Generate` (lines 120-126)
Change from:
```go
	slices.SortStableFunc(records, func(a, b store.ContainerRecord) int {
		return cmp.Or(
			g.localMachineRank(a.MachineID)-g.localMachineRank(b.MachineID),
			strings.Compare(a.Container.ServiceName(), b.Container.ServiceName()),
			a.Container.CreatedTime().Compare(b.Container.CreatedTime()),
		)
	})
```

To:
```go
	slices.SortStableFunc(records, func(a, b store.ContainerRecord) int {
		return cmp.Or(
			cmp.Compare(g.rttForMachine(a.MachineID), g.rttForMachine(b.MachineID)),
			strings.Compare(a.Container.ServiceName(), b.Container.ServiceName()),
			a.Container.CreatedTime().Compare(b.Container.CreatedTime()),
		)
	})
```

#### 4e: Replace `localMachineRank` with `rttForMachine` (lines 252-259)
Delete the entire `localMachineRank` method:
```go
// localMachineRank returns 0 if the given machineID matches the local machine and 1 otherwise.
// Useful for sorting containers running locally first.
func (g *CaddyfileGenerator) localMachineRank(machineID string) int {
	if g.machineID == machineID {
		return 0
	}
	return 1
}
```

Replace it with:
```go
// rttForMachine returns the RTT to a given machine. Returns 0 for the local machine
// (same machine as the generator), the median RTT for known remote machines, and
// math.MaxInt64 for unknown machines (sorted last).
func (g *CaddyfileGenerator) rttForMachine(machineID string) time.Duration {
	if g.machineID == machineID {
		return 0
	}
	if g.rttByMachineID != nil {
		if rtt, ok := g.rttByMachineID(machineID); ok {
			return rtt
		}
	}
	return time.Duration(math.MaxInt64)
}
```

---

> ✅ **Step 5: Modify `internal/machine/caddyconfig/controller.go`**
> - Add `"time"` import, add `rttByMachineID` field to `Controller`, update `NewController` signature, update `Run` to pass `rttByMachineID` to `NewCaddyfileGenerator`.
> - Expected outcome: Caddy controller threads RTT data through to the generator.

#### 5a: Add `"time"` to imports (lines 3-17)
The current import block:
```go
import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/psviderski/uncloud/internal/fs"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
)
```

Add `"time"` alphabetically:
```go
import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/psviderski/uncloud/internal/fs"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
)
```

#### 5b: Add `rttByMachineID` field to `Controller` struct (lines 28-40)
Add the `rttByMachineID` field:

```go
type Controller struct {
	machineID      string
	caddyfilePath  string
	generator      *CaddyfileGenerator
	client         *CaddyAdminClient
	store          *store.Store
	rttByMachineID func(string) (time.Duration, bool)
	log            *slog.Logger
	// lastFingerprint caches the fingerprint of the containers used to generate the latest successfully loaded
	// Caddyfile. nil means it hasn't been loaded yet or the last load failed.
	lastFingerprint []containerFingerprint
	// lastCaddyfile caches the last generated Caddyfile.
	lastCaddyfile string
}
```

#### 5c: Update `NewController` (lines 59-78)
Change from:
```go
func NewController(machineID, configDir, adminSock string, store *store.Store) (*Controller, error) {
```

To:
```go
func NewController(machineID, configDir, adminSock string, store *store.Store, rttByMachineID func(string) (time.Duration, bool)) (*Controller, error) {
```

And add `rttByMachineID:` to the return struct (lines 71-77):
```go
	return &Controller{
		machineID:      machineID,
		caddyfilePath:  filepath.Join(configDir, "Caddyfile"),
		client:         client,
		store:          store,
		rttByMachineID: rttByMachineID,
		log:            log,
	}, nil
```

#### 5d: Update `Run` method to pass `rttByMachineID` to `NewCaddyfileGenerator` (line 90)
Change from:
```go
		c.generator = NewCaddyfileGenerator(c.machineID, machineName, c.client, c.log)
```

To:
```go
		c.generator = NewCaddyfileGenerator(c.machineID, machineName, c.rttByMachineID, c.client, c.log)
```

---

> ✅ **Step 6: Modify `internal/machine/machine.go`**
> - Add `rttCache` field to `Machine` struct, create and store RTT cache, pass `rttCache.ByMachineID` to DNS server and Caddy controller, start RTT cache in errGroup, simplify `getMachineRTTs`.
> - Expected outcome: Machine orchestrates RTT cache lifecycle and provides RTT data to DNS and Caddy.

#### 6a: Add `rttCache` field to `Machine` struct (after line 179)
Add the field after `cluster *cluster.Cluster`:

```go
	cluster *cluster.Cluster
	// rttCache caches round-trip time statistics to other machines in the cluster.
	rttCache *RTTCache
	// store is the cluster store backed by a distributed Corrosion database.
	store   *store.Store
```

#### 6b: Create RTT cache and pass to DNS server and Caddy controller (inside `<-m.initialised` case, lines ~443-464)
The new parameter needs to be added to both the Caddy controller and DNS server constructors. The `rttCache` must be created before the Caddy controller (line 445) and DNS server (line 456).

Add the RTT cache creation BEFORE the Caddy controller creation (before line 445):
```go
			rttCache := newRTTCache(m.state.ID, m.cluster, m.store)
```

Update `NewController` call (lines 445-450) from:
```go
			caddyconfigCtrl, err := caddyconfig.NewController(
				m.state.ID,
				m.config.CaddyConfigDir,
				DefaultCaddyAdminSockPath,
				m.store,
			)
```

To:
```go
			caddyconfigCtrl, err := caddyconfig.NewController(
				m.state.ID,
				m.config.CaddyConfigDir,
				DefaultCaddyAdminSockPath,
				m.store,
				rttCache.ByMachineID,
			)
```

Update `NewServer` call (lines 456-464) from:
```go
			dnsServer, err := dns.NewServer(
				m.IP(),
				m.state.Network.Subnet,
				dnsResolver,
				m.config.DNSUpstreams,
			)
```

To:
```go
			dnsServer, err := dns.NewServer(
				m.IP(),
				m.state.Network.Subnet,
				dnsResolver,
				m.config.DNSUpstreams,
				rttCache.ByMachineID,
			)
```

#### 6c: Store `rttCache` on the Machine struct (before line 493)
Add `m.rttCache = rttCache` before the `m.mu.Lock()` line:

```go
			m.rttCache = rttCache

			m.mu.Lock()
			m.clusterCtrl, err = newClusterController(
```

#### 6d: Start RTT cache in errGroup (before `m.clusterCtrl.Run(ctx)` at line 512)
Add an errGroup goroutine to start the RTT cache before the blocking `m.clusterCtrl.Run(ctx)` call:

```go
			// Start the RTT cache for periodic refresh of cluster RTT data.
			errGroup.Go(func() error {
				if err := rttCache.Run(ctx); err != nil {
					return fmt.Errorf("run RTT cache: %w", err)
				}
				return nil
			})

			if err = m.clusterCtrl.Run(ctx); err != nil {
```

#### 6e: Simplify `getMachineRTTs` method (lines 948-980)
Replace the entire method:

From:
```go
// getMachineRTTs retrieves round-trip times to other machines in the cluster.
func (m *Machine) getMachineRTTs(ctx context.Context) (map[string]*pb.RTTStats, error) {
	rtts, err := m.cluster.MemberRTTs()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get member rtts: %v", err)
	}

	// List machines to map IPs to Machine IDs.
	machines, err := m.store.ListMachines(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list machines: %v", err)
	}

	// Map Management IP -> Machine ID
	ipToMachineID := make(map[netip.Addr]string)
	for _, mach := range machines {
		ip, _ := mach.Network.ManagementIp.ToAddr()
		ipToMachineID[ip] = mach.Id
	}

	pbRTTs := make(map[string]*pb.RTTStats)
	for _, stats := range rtts {
		// Corrosion uses the management IP for gossip.
		if mid, ok := ipToMachineID[stats.Addr.Addr()]; ok {
			pbRTTs[mid] = &pb.RTTStats{
				Median: durationpb.New(stats.Median),
				StdDev: durationpb.New(stats.StdDev),
			}
		}
	}

	return pbRTTs, nil
}
```

To:
```go
// getMachineRTTs retrieves round-trip times to other machines in the cluster from the RTT cache.
func (m *Machine) getMachineRTTs() (map[string]*pb.RTTStats, error) {
	if m.rttCache == nil {
		return nil, nil
	}

	all := m.rttCache.All()
	pbRTTs := make(map[string]*pb.RTTStats, len(all))
	for mid, stats := range all {
		pbRTTs[mid] = &pb.RTTStats{
			Median: durationpb.New(stats.Median),
			StdDev: durationpb.New(stats.StdDev),
		}
	}
	return pbRTTs, nil
}
```

**Note:** The `ctx context.Context` parameter is removed since the method no longer needs it (it reads from the in-memory cache). The nil check on `m.rttCache` handles the case where `InspectMachine` is called before initialization completes.

#### 6f: Update `InspectMachine` call site (lines 920-926)
Since `getMachineRTTs` no longer takes a context parameter, update the call:

From:
```go
	var rtts map[string]*pb.RTTStats
	if m.Initialised() {
		rtts, err = m.getMachineRTTs(ctx)
		if err != nil {
			return nil, err
		}
	}
```

To:
```go
	var rtts map[string]*pb.RTTStats
	if m.Initialised() {
		rtts, err = m.getMachineRTTs()
		if err != nil {
			return nil, err
		}
	}
```

#### 6g: Clean up unused imports
After the `getMachineRTTs` simplification, check if `"net/netip"` is still used elsewhere in `machine.go`. It IS still used (e.g., in `IP()` method and other places), so keep it. The `corrosion` import is also still used (for the `cluster` package). However, `net/netip` might no longer be needed ONLY by `getMachineRTTs` — verify it's used elsewhere before removing. Since `machine.go` uses `netip` in many places, do NOT remove it.

The import `"github.com/psviderski/uncloud/internal/corrosion"` — check if it's still used after removing `getMachineRTTs`'s direct Corrosion call. Since the import is for the `cluster` package which uses `corrosion`, and there may be other references, do NOT remove it without verifying. Actually, looking at the imports, the `corrosion` import is at `github.com/psviderski/uncloud/internal/corrosion`. Searching for `corrosion` in machine.go... it's used in other methods. Keep it.

---

> ✅ **Step 7: Update test files**
> - Update `NewCaddyfileGenerator` call sites in `caddyfile_test.go` to pass nil `rttByMachineID`. Verify `resolver_test.go` compiles with new `ResolvedIP` type.
> - Expected outcome: All tests pass with the new signatures.

#### 7a: Update `internal/machine/dns/resolver_test.go`
The test at `TestClusterResolver_UpdateServiceIPs` uses `r.serviceIPs` directly and `r.Resolve()`. Both need to account for the type change from `[]netip.Addr` to `[]ResolvedIP`.

The `newRecord` helper (line 51-76) creates `store.ContainerRecord` with `MachineID` field already set, so the `MachineID` will be present in `ResolvedIP` results.

Changes needed:
1. Line 26: `r.Resolve("web")` now returns `[]ResolvedIP` instead of `[]netip.Addr`. The assertion `assert.NotEmpty(t, r.Resolve("web"))` still works because `[]ResolvedIP` is not empty.
2. Line 29: `r.Resolve("api")` — same.
3. Lines 26-27: `reflect.ValueOf(r.serviceIPs).Pointer()` — this still works since `serviceIPs` is still a map.
4. Line 48: `r.Resolve("db")` — same pattern.

The test should compile and pass without changes because:
- `assert.NotEmpty(t, r.Resolve("web"))` works for `[]ResolvedIP` just as for `[]netip.Addr`.
- `reflect.ValueOf(r.serviceIPs).Pointer()` works for any map type.
- The `newRecord` helper already sets `MachineID`.

However, verify the test compiles after the changes. If `Maps.EqualFunc` was used in the test, it would need updating, but looking at the test code, it doesn't use `maps`.

#### 7b: Update `internal/machine/caddyconfig/caddyfile_test.go`
Three call sites need the new `rttByMachineID` parameter added to `NewCaddyfileGenerator`:

1. Line 212: `NewCaddyfileGenerator("test-machine-id", "test-machine", nil, nil)` → `NewCaddyfileGenerator("test-machine-id", "test-machine", nil, nil, nil)`

2. Line 856: `NewCaddyfileGenerator("test-machine-id", "test-machine", validator, nil)` → `NewCaddyfileGenerator("test-machine-id", "test-machine", nil, validator, nil)`

3. Line 998: `NewCaddyfileGenerator("test-machine-id", "test-machine", nil, nil)` → `NewCaddyfileGenerator("test-machine-id", "test-machine", nil, nil, nil)`

Since `rttByMachineID` is nil, `rttForMachine` returns `time.Duration(math.MaxInt64)` for all remote machines. This produces the same sort order as the old `localMachineRank` (0 for local, maxInt64 for remote), so all existing tests should pass with the same expected output.

---

> ✅ **Step 8: Verify compilation and run tests**
> - Build and test the modified packages to ensure everything compiles and tests pass.
> - Expected outcome: `go build` and `go test` pass without errors.

After all changes, verify:

```bash
go build ./internal/machine/...
go test ./internal/machine/dns/...
go test ./internal/machine/caddyconfig/...
```

These commands should compile and pass without errors.