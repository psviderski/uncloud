package dns

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
)

// ClusterResolver implements Resolver by tracking containers in the cluster and resolving service names
// to their IP addresses.
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

// NewClusterResolver creates a new cluster resolver using the cluster store.
func NewClusterResolver(store *store.Store) *ClusterResolver {
	return &ClusterResolver{
		store:      store,
		serviceIPs: make(map[string][]netip.Addr),
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
	newServiceIPs := make(map[string][]netip.Addr, len(r.serviceIPs))

	containersCount := 0
	for _, record := range containers {
		if !record.Container.Healthy() {
			continue
		}

		ip := record.Container.UncloudNetworkIP()
		if !ip.IsValid() {
			// Container is not connected to the uncloud Docker network (could be host network).
			continue
		}

		ctr := api.ServiceContainer{Container: record.Container}
		if ctr.ServiceID() == "" || ctr.ServiceName() == "" {
			// Container is not part of a service, skip it.
			continue
		}

		// TODO: remove normalisation after implementing service name validation:
		//.https://github.com/psviderski/uncloud/issues/53
		serviceName := strings.ToLower(ctr.ServiceName())

		newServiceIPs[serviceName] = append(newServiceIPs[serviceName], ip)
		// Also add the service ID as a valid lookup.
		newServiceIPs[ctr.ServiceID()] = append(newServiceIPs[ctr.ServiceID()], ip)
		containersCount++
	}

	// Update the serviceIPs map atomically.
	r.mu.Lock()
	r.serviceIPs = newServiceIPs
	r.mu.Unlock()

	r.log.Debug("DNS records updated.", "services", len(newServiceIPs)/2, "containers", containersCount)
}

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
