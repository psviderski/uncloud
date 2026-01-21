package dns

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
)

// resolverMaps holds DNS resolution data that must be updated atomically.
type resolverMaps struct {
	serviceIPs   map[string]map[string][]netip.Addr // namespace -> service name -> IPs
	containerIPs map[netip.Addr]string              // IP -> namespace
}

// ClusterResolver implements Resolver by tracking containers in the cluster and resolving service names
// to their IP addresses.
type ClusterResolver struct {
	store *store.Store
	maps  *resolverMaps
	mu    sync.RWMutex
	// lastUpdate tracks when records were last updated.
	lastUpdate time.Time
	log        *slog.Logger
}

// NewClusterResolver creates a new cluster resolver using the cluster store.
func NewClusterResolver(store *store.Store) *ClusterResolver {
	return &ClusterResolver{
		store: store,
		maps: &resolverMaps{
			serviceIPs:   make(map[string]map[string][]netip.Addr),
			containerIPs: make(map[netip.Addr]string),
		},
		log: slog.With("component", "dns-resolver"),
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
	r.mu.RLock()
	currentMaps := r.maps
	r.mu.RUnlock()

	newServiceIPs := make(map[string]map[string][]netip.Addr, len(currentMaps.serviceIPs))
	newContainerIPs := make(map[netip.Addr]string)

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

		ctr := record.Container
		if ctr.ServiceID() == "" || ctr.ServiceName() == "" {
			// Container is not part of a service, skip it.
			continue
		}

		namespace := ctr.Namespace()
		if newServiceIPs[namespace] == nil {
			newServiceIPs[namespace] = make(map[string][]netip.Addr)
		}

		newContainerIPs[ip] = namespace
		newServiceIPs[namespace][ctr.ServiceName()] = append(newServiceIPs[namespace][ctr.ServiceName()], ip)
		// Also add the service ID as a valid lookup.
		newServiceIPs[namespace][ctr.ServiceID()] = append(newServiceIPs[namespace][ctr.ServiceID()], ip)

		// Add <machine-id>.m.<service-name> as a lookup
		serviceNameWithMachineID := record.MachineID + ".m." + ctr.ServiceName()
		newServiceIPs[namespace][serviceNameWithMachineID] = append(newServiceIPs[namespace][serviceNameWithMachineID], ip)

		containersCount++
	}

	// Update both maps atomically by replacing the entire resolverMaps pointer.
	r.mu.Lock()
	r.maps = &resolverMaps{
		serviceIPs:   newServiceIPs,
		containerIPs: newContainerIPs,
	}
	r.mu.Unlock()

	r.log.Debug("DNS records updated.", "namespaces", len(newServiceIPs), "containers", containersCount)
}

// Resolve returns IP addresses of the service containers.
func (r *ClusterResolver) Resolve(serviceName string, namespace string) []netip.Addr {
	r.mu.RLock()
	maps := r.maps
	r.mu.RUnlock()

	if namespace == "" {
		namespace = api.DefaultNamespace
	}

	ips, ok := maps.serviceIPs[namespace][serviceName]
	if !ok || len(ips) == 0 {
		return nil
	}

	// Return a copy of the IPs slice to prevent modification of the original.
	ipsCopy := make([]netip.Addr, len(ips))
	copy(ipsCopy, ips)

	return ipsCopy
}

// GetNamespaceByIP returns the namespace a given IP belongs to.
func (r *ClusterResolver) GetNamespaceByIP(ip netip.Addr) string {
	r.mu.RLock()
	maps := r.maps
	r.mu.RUnlock()
	return maps.containerIPs[ip]
}
