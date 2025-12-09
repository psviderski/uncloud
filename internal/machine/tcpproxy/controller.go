package tcpproxy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
)

// Controller monitors container changes in the cluster store and updates the TCP proxy
// to route traffic for TCP ingress ports.
type Controller struct {
	proxy *Proxy
	store *store.Store
	log   *slog.Logger
}

// NewController creates a new TCP proxy controller.
func NewController(proxy *Proxy, store *store.Store) *Controller {
	return &Controller{
		proxy: proxy,
		store: store,
		log:   slog.With("component", "tcp-proxy-controller"),
	}
}

// Run starts the controller and blocks until the context is cancelled.
func (c *Controller) Run(ctx context.Context) error {
	containers, changes, err := c.store.SubscribeContainers(ctx)
	if err != nil {
		return fmt.Errorf("subscribe to container changes: %w", err)
	}
	c.log.Info("Subscribed to container changes for TCP proxy configuration.")

	containers = filterHealthyContainers(containers)
	c.updateProxy(ctx, containers)

	for {
		select {
		case _, ok := <-changes:
			if !ok {
				return fmt.Errorf("containers subscription failed")
			}
			c.log.Info("Cluster containers changed, updating TCP proxy configuration.")

			containers, err = c.store.ListContainers(ctx, store.ListOptions{})
			if err != nil {
				c.log.Error("Failed to list containers.", "err", err)
				continue
			}
			containers = filterHealthyContainers(containers)
			c.updateProxy(ctx, containers)

		case <-ctx.Done():
			return nil
		}
	}
}

// updateProxy extracts TCP ingress ports from containers and updates the proxy.
func (c *Controller) updateProxy(ctx context.Context, containers []store.ContainerRecord) {
	// Build port → backends mapping
	portBackends := make(map[uint16][]string)

	for _, cr := range containers {
		ports, err := cr.Container.ServicePorts()
		if err != nil {
			c.log.Error("Failed to get service ports.", "container", cr.Container.ShortID(), "err", err)
			continue
		}

		ip := cr.Container.UncloudNetworkIP()
		if !ip.IsValid() {
			continue
		}

		for _, port := range ports {
			// Only handle TCP ingress ports
			if port.Protocol != api.ProtocolTCP {
				continue
			}
			if port.Mode != "" && port.Mode != api.PortModeIngress {
				continue
			}
			if port.PublishedPort == 0 {
				// No published port assigned yet
				continue
			}

			backend := fmt.Sprintf("%s:%d", ip, port.ContainerPort)
			portBackends[port.PublishedPort] = append(portBackends[port.PublishedPort], backend)
		}
	}

	// Get current ports from proxy
	currentPorts := make(map[uint16]struct{})
	for _, port := range c.proxy.ListeningPorts() {
		currentPorts[port] = struct{}{}
	}

	// Update proxy with new backends
	for port, backends := range portBackends {
		if err := c.proxy.SetBackends(ctx, port, backends); err != nil {
			c.log.Error("Failed to set backends for port.", "port", port, "err", err)
		}
		delete(currentPorts, port)
	}

	// Remove ports that no longer have any TCP services
	for port := range currentPorts {
		if err := c.proxy.RemovePort(port); err != nil {
			c.log.Error("Failed to remove port.", "port", port, "err", err)
		}
	}

	c.log.Info("TCP proxy updated.", "ports", len(portBackends))
}

// filterHealthyContainers filters out containers that are not healthy.
func filterHealthyContainers(containers []store.ContainerRecord) []store.ContainerRecord {
	healthy := make([]store.ContainerRecord, 0, len(containers))
	for _, cr := range containers {
		if cr.Container.Healthy() {
			healthy = append(healthy, cr)
		}
	}
	return healthy
}
