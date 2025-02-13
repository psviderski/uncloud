package api

import (
	"fmt"
	"github.com/docker/docker/api/types"
	"regexp"
	"strings"
)

const (
	LabelManaged      = "uncloud.managed"
	LabelServiceID    = "uncloud.service.id"
	LabelServiceName  = "uncloud.service.name"
	LabelServiceMode  = "uncloud.service.mode"
	LabelServicePorts = "uncloud.service.ports"

	StateCreated    = "created"
	StateDead       = "dead"
	StateExited     = "exited"
	StatePaused     = "paused"
	StateRemoving   = "removing"
	StateRestarting = "restarting"
	StateRunning    = "running"
)

type Container struct {
	types.Container
}

func (c *Container) Name() string {
	return c.Names[0][1:] // Remove leading slash.
}

// ServiceID returns the ID of the service this container belongs to.
func (c *Container) ServiceID() string {
	return c.Labels[LabelServiceID]
}

// ServiceName returns the name of the service this container belongs to.
func (c *Container) ServiceName() string {
	return c.Labels[LabelServiceName]
}

// ServiceMode returns the replication mode of the service this container belongs to.
func (c *Container) ServiceMode() string {
	return c.Labels[LabelServiceMode]
}

// ServicePorts returns the ports this container publishes as part of its service.
func (c *Container) ServicePorts() ([]PortSpec, error) {
	encoded, ok := c.Labels[LabelServicePorts]
	if !ok {
		return nil, nil
	}
	if strings.TrimSpace(encoded) == "" {
		return nil, nil
	}

	publishPorts := strings.Split(encoded, ",")
	ports := make([]PortSpec, len(publishPorts))
	for i, p := range publishPorts {
		port, err := ParsePortSpec(strings.TrimSpace(p))
		if err != nil {
			return nil, err
		}
		ports[i] = port
	}

	return ports, nil
}

func (c *Container) ServiceSpec() ServiceSpec {
	// TODO: migrate api.Container type to use ContainerJSON to make it possible to construct
	// a ServiceSpec from a Container.
	return ServiceSpec{}
}

// runningStatusRegex matches the status string of a running container.
// - "Up 3 minutes (healthy)" -> groups: ["Up 3 minutes (healthy)", "healthy"]
// - "Up 5 seconds" -> groups: ["Up 5 seconds", ""]
// - "Up 2 hours (unhealthy)" -> groups: ["Up 2 hours (unhealthy)", "unhealthy"]
// - "Up 1 minute (health: starting)" -> groups: ["Up 1 minute (health: starting)", "health: starting"]
// - "Restarting (0) 5 seconds ago" -> no match
// See https://github.com/moby/moby/blob/c130ce1f5d1e38b98a97044a39557de43bc0d58f/container/state.go#L77-L90
// for more details on how the status string for a running container is formatted.
var runningStatusRegex = regexp.MustCompile(`^Up [^(]+(?:\(([^)]+)\))?$`)

// Healthy determines if the container is running and healthy based on its status string.
// A running container with no health check configured is considered healthy.
func (c *Container) Healthy() bool {
	if c.State != StateRunning {
		return false
	}

	matches := runningStatusRegex.FindStringSubmatch(c.Status)
	// Not "Up" or invalid format.
	if matches == nil {
		return false
	}

	// If there's no health status (no health check configured so no parentheses), container is considered healthy.
	if matches[1] == "" {
		return true
	}

	// If the health status in parentheses is "healthy", the container is considered healthy.
	return matches[1] == types.Healthy
}

// Stopped determines if the container is stopped and doesn't try to restart.
func (c *Container) Stopped() bool {
	return c.State == StateCreated || c.State == StateDead || c.State == StateExited
}

// ConflictingServicePorts returns a list of service ports that conflict with the given ports.
func (c *Container) ConflictingServicePorts(ports []PortSpec) ([]PortSpec, error) {
	svcPorts, err := c.ServicePorts()
	if err != nil {
		return nil, fmt.Errorf("get service ports: %w", err)
	}

	var conflicting []PortSpec
	for _, p := range ports {
		if p.Mode != PortModeHost {
			continue
		}

		// Two host ports conflict if they have the same published port number and protocol, and either:
		//   * At least one host IP is not set (meaning it uses all interfaces)
		//   * Both host IPs are identical
		for _, svcPort := range svcPorts {
			if svcPort.Mode != PortModeHost ||
				svcPort.PublishedPort != p.PublishedPort ||
				svcPort.Protocol != p.Protocol {
				continue
			}

			if !svcPort.HostIP.IsValid() || !p.HostIP.IsValid() || svcPort.HostIP.Compare(p.HostIP) == 0 {
				conflicting = append(conflicting, p)
			}
		}
	}

	return conflicting, nil
}
