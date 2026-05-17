package dns

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/netip"
	"slices"
	"sync"
	"time"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/psviderski/uncloud/internal/machine/store"
)

// ClusterResolver implements Resolver by tracking containers in the cluster and resolving service names
// to their IP addresses.
type ClusterResolver struct {
	store *store.Store
	// serviceIPs maps service names to container IPs.
	serviceIPs map[string][]netip.Addr
	// machineIPs maps machine IDs and names to their IP.
	machineIPs map[string]netip.Addr
	// mu protects the serviceIPs and machineIPs map.
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

	machines, mchanges, err := r.store.SubscribeMachines(ctx)
	if err != nil {
		return fmt.Errorf("subscribe to machine changes: %w", err)
	}
	r.log.Info("Subscribed to machine changes in the clsuter to keep machine DNS records updated.")
	r.updateMachineIPs(machines)

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

		case _, ok := <-mchanges:
			if !ok {
				return fmt.Errorf("machine subscription failed")
			}
			r.log.Debug("Cluster machines changed, updating DNS records.")

			machines, err := r.store.ListMachines(ctx)
			if err != nil {
				r.log.Error("Failed to list machines.", "err", err)
				continue
			}
			r.updateMachineIPs(machines)

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

		newServiceIPs[ctr.ServiceName()] = append(newServiceIPs[ctr.ServiceName()], ip)
		// Also add the service ID as a valid lookup.
		newServiceIPs[ctr.ServiceID()] = append(newServiceIPs[ctr.ServiceID()], ip)

		// Add <machine-id>.m.<service-name> as a lookup
		serviceNameWithMachineID := record.MachineID + ".m." + ctr.ServiceName()
		newServiceIPs[serviceNameWithMachineID] = append(newServiceIPs[serviceNameWithMachineID], ip)

		containersCount++
	}

	// Sort each service's IPs so they have a deterministic order for comparison.
	for _, ips := range newServiceIPs {
		slices.SortFunc(ips, func(a, b netip.Addr) int { return a.Compare(b) })
	}
	// Skip the swap when the services or their container IPs haven't changed.
	if maps.EqualFunc(r.serviceIPs, newServiceIPs, slices.Equal[[]netip.Addr]) {
		return
	}

	// Update the serviceIPs map atomically.
	r.mu.Lock()
	r.serviceIPs = newServiceIPs
	r.mu.Unlock()

	r.log.Info("DNS records updated.", "services", len(newServiceIPs)/3, "containers", containersCount)
}

func (r *ClusterResolver) updateMachineIPs(machines []*pb.MachineInfo) {
	newMachineIPs := make(map[string]netip.Addr, len(machines))

	for _, machine := range machines {
		subnet, err := machine.Network.Subnet.ToPrefix()
		if err != nil {
			continue
		}
		addr := network.MachineIP(subnet)
		newMachineIPs[machine.Name+".m"] = addr
		newMachineIPs[machine.Id+".m"] = addr
	}
	r.mu.Lock()
	r.machineIPs = newMachineIPs
	r.mu.Unlock()

	r.log.Info("DNS records updated.", "machines", len(machines))
}

// Resolve returns IP addresses of the service containers or machines.
func (r *ClusterResolver) Resolve(name string) []netip.Addr {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy of the IPs slice to prevent modification of the original.
	ips, ok := r.serviceIPs[name]
	if ok && len(ips) > 0 {
		return slices.Clone(ips)
	}

	ip, ok := r.machineIPs[name]
	if ok {
		return slices.Clone([]netip.Addr{ip})
	}

	return nil
}
