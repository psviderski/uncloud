package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
)

// Controller monitors container changes in the cluster store and updates TCP/UDP proxies
// to route traffic for ingress ports.
type Controller struct {
	store *store.Store
	log   *slog.Logger

	mu         sync.Mutex
	tcpProxies map[uint16]*TCPProxy
	udpProxies map[uint16]*UDPProxy
}

// NewController creates a new proxy controller.
func NewController(store *store.Store, log *slog.Logger) *Controller {
	if log == nil {
		log = slog.Default()
	}
	return &Controller{
		store:      store,
		log:        log.With("component", "proxy-controller"),
		tcpProxies: make(map[uint16]*TCPProxy),
		udpProxies: make(map[uint16]*UDPProxy),
	}
}

// Run starts the controller and blocks until the context is cancelled.
func (c *Controller) Run(ctx context.Context) error {
	containers, changes, err := c.store.SubscribeContainers(ctx)
	if err != nil {
		return fmt.Errorf("subscribe to container changes: %w", err)
	}
	c.log.Info("Subscribed to container changes for proxy configuration.")

	containers = filterHealthyContainers(containers)
	c.updateProxies(ctx, containers)

	for {
		select {
		case _, ok := <-changes:
			if !ok {
				return fmt.Errorf("containers subscription failed")
			}
			c.log.Info("Cluster containers changed, updating proxy configuration.")

			containers, err = c.store.ListContainers(ctx, store.ListOptions{})
			if err != nil {
				c.log.Error("Failed to list containers.", "err", err)
				continue
			}
			containers = filterHealthyContainers(containers)
			c.updateProxies(ctx, containers)

		case <-ctx.Done():
			c.closeAll()
			return nil
		}
	}
}

// updateProxies extracts ingress ports from containers and updates TCP/UDP proxies.
func (c *Controller) updateProxies(ctx context.Context, containers []store.ContainerRecord) {
	// Build port → backends mappings for TCP and UDP
	tcpBackends := make(map[uint16][]string)
	udpBackends := make(map[uint16][]string)

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
			// Only handle ingress ports
			if port.Mode != "" && port.Mode != api.PortModeIngress {
				continue
			}
			if port.PublishedPort == 0 {
				// No published port assigned yet
				continue
			}

			backend := fmt.Sprintf("%s:%d", ip, port.ContainerPort)

			switch port.Protocol {
			case api.ProtocolTCP:
				tcpBackends[port.PublishedPort] = append(tcpBackends[port.PublishedPort], backend)
			case api.ProtocolUDP:
				udpBackends[port.PublishedPort] = append(udpBackends[port.PublishedPort], backend)
			}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Update TCP proxies
	c.updateTCPProxies(ctx, tcpBackends)

	// Update UDP proxies
	c.updateUDPProxies(ctx, udpBackends)

	c.log.Info("Proxies updated.", "tcp_ports", len(tcpBackends), "udp_ports", len(udpBackends))
}

func (c *Controller) updateTCPProxies(_ context.Context, portBackends map[uint16][]string) {
	// Track current ports
	currentPorts := make(map[uint16]struct{})
	for port := range c.tcpProxies {
		currentPorts[port] = struct{}{}
	}

	// Update or create proxies
	for port, backends := range portBackends {
		proxy, exists := c.tcpProxies[port]
		if !exists {
			var err error
			proxy, err = NewTCPProxy(port, c.log.With("protocol", "tcp", "port", port))
			if err != nil {
				c.log.Error("Failed to create TCP proxy.", "port", port, "err", err)
				continue
			}
			c.tcpProxies[port] = proxy
			go proxy.Run()
			c.log.Info("Started TCP proxy.", "port", port)
		}
		proxy.SetBackends(backends)
		delete(currentPorts, port)
	}

	// Remove proxies for ports that no longer have backends
	for port := range currentPorts {
		if proxy, ok := c.tcpProxies[port]; ok {
			proxy.Close()
			delete(c.tcpProxies, port)
			c.log.Info("Stopped TCP proxy.", "port", port)
		}
	}
}

func (c *Controller) updateUDPProxies(_ context.Context, portBackends map[uint16][]string) {
	// Track current ports
	currentPorts := make(map[uint16]struct{})
	for port := range c.udpProxies {
		currentPorts[port] = struct{}{}
	}

	// Update or create proxies
	for port, backends := range portBackends {
		proxy, exists := c.udpProxies[port]
		if !exists {
			var err error
			proxy, err = NewUDPProxy(port, c.log.With("protocol", "udp", "port", port))
			if err != nil {
				c.log.Error("Failed to create UDP proxy.", "port", port, "err", err)
				continue
			}
			c.udpProxies[port] = proxy
			go proxy.Run()
			c.log.Info("Started UDP proxy.", "port", port)
		}
		proxy.SetBackends(backends)
		delete(currentPorts, port)
	}

	// Remove proxies for ports that no longer have backends
	for port := range currentPorts {
		if proxy, ok := c.udpProxies[port]; ok {
			proxy.Close()
			delete(c.udpProxies, port)
			c.log.Info("Stopped UDP proxy.", "port", port)
		}
	}
}

func (c *Controller) closeAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for port, proxy := range c.tcpProxies {
		proxy.Close()
		delete(c.tcpProxies, port)
	}
	for port, proxy := range c.udpProxies {
		proxy.Close()
		delete(c.udpProxies, port)
	}
	c.log.Info("All proxies closed.")
}

// ListeningTCPPorts returns a list of ports the TCP proxies are listening on.
func (c *Controller) ListeningTCPPorts() []uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()

	ports := make([]uint16, 0, len(c.tcpProxies))
	for port := range c.tcpProxies {
		ports = append(ports, port)
	}
	return ports
}

// ListeningUDPPorts returns a list of ports the UDP proxies are listening on.
func (c *Controller) ListeningUDPPorts() []uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()

	ports := make([]uint16, 0, len(c.udpProxies))
	for port := range c.udpProxies {
		ports = append(ports, port)
	}
	return ports
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
