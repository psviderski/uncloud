package dns

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/psviderski/uncloud/internal/machine/store"
)

// ClusterResolver implements Resolver by tracking containers in the cluster and resolving service names
// to their IP addresses.
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

// NewClusterResolver creates a new cluster resolver using the cluster store.
func NewClusterResolver(store *store.Store) *ClusterResolver {
	return &ClusterResolver{
		store:      store,
		serviceIPs: make(map[string][]ResolvedIP),
		log:        slog.With("component", "dns-resolver"),
	}
}

// Run starts watching for container changes and updates DNS records accordingly.
func (r *ClusterResolver) Run(ctx context.Context) error {
	containers, changes, err := r.store.SubscribeContainers(ctx)
	if err != nil {
		return fmt.Errorf("subscribe to container changes: %w", err)
	}
	r.log.Info("Subscribed to container changes in the cluster to keep DNS records updated.")

	// TODO: implement machine membership check using Corrossion Admin client to filter available containers.
	r.updateServiceIPs(containers)

	for {
		select {
		case _, ok := <-changes:
			if !ok {
				return fmt.Errorf("containers subscription failed")
			}
			r.log.Debug("Cluster containers changed, updating DNS records.")

			containers, err = r.store.ListContainers(ctx, store.ListOptions{})
			if err != nil {
				r.log.Error("Failed to list containers.", "err", err)
				continue
			}

			// TODO: implement machine membership check using Corrossion Admin client to filter available containers.
			r.updateServiceIPs(containers)
		case <-ctx.Done():
			return nil
		}
	}
}

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
