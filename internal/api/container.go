package api

import (
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/go-units"
	"strings"
	"time"
)

const (
	LabelManaged      = "uncloud.managed"
	LabelServiceID    = "uncloud.service.id"
	LabelServiceName  = "uncloud.service.name"
	LabelServiceMode  = "uncloud.service.mode"
	LabelServicePorts = "uncloud.service.ports"
)

type Container struct {
	types.ContainerJSON
}

// NameWithoutSlash returns the container name without the leading slash.
func (c *Container) NameWithoutSlash() string {
	return c.Name[1:]
}

// ServiceID returns the ID of the service this container belongs to.
func (c *Container) ServiceID() string {
	return c.Config.Labels[LabelServiceID]
}

// ServiceName returns the name of the service this container belongs to.
func (c *Container) ServiceName() string {
	return c.Config.Labels[LabelServiceName]
}

// ServiceMode returns the replication mode of the service this container belongs to.
func (c *Container) ServiceMode() string {
	return c.Config.Labels[LabelServiceMode]
}

// ServicePorts returns the ports this container publishes as part of its service.
func (c *Container) ServicePorts() ([]PortSpec, error) {
	encoded, ok := c.Config.Labels[LabelServicePorts]
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

// ServiceSpec constructs a service spec from the container's configuration.
func (c *Container) ServiceSpec() (ServiceSpec, error) {
	ports, err := c.ServicePorts()
	if err != nil {
		return ServiceSpec{}, fmt.Errorf("get service ports: %w", err)
	}

	return ServiceSpec{
		Container: ContainerSpec{
			Command: c.Config.Cmd,
			Image:   c.Config.Image,
			Init:    c.HostConfig.Init,
			Volumes: c.HostConfig.Binds,
		},
		Mode:  c.ServiceMode(),
		Name:  c.ServiceName(),
		Ports: ports,
	}, nil
}

// Healthy determines if the container is running and healthy.
// A running container with no health check configured is considered healthy.
func (c *Container) Healthy() bool {
	if !c.State.Running || c.State.Paused || c.State.Restarting {
		return false
	}

	// If there's no health status (no health check configured), container is considered healthy.
	if c.State.Health == nil {
		return true
	}

	return c.State.Health.Status == types.Healthy
}

// HumanState returns a human-readable description of the container's state. Based on the Docker implementation:
// https://github.com/moby/moby/blob/b343d235a0a1f30c8f05b1d651238e72158dc25d/container/state.go#L79-L113
func (c *Container) HumanState() (string, error) {
	startedAt, err := time.Parse(time.RFC3339Nano, c.State.StartedAt)
	if err != nil {
		return "", fmt.Errorf("parse started time: %w", err)
	}
	finishedAt, err := time.Parse(time.RFC3339Nano, c.State.FinishedAt)
	if err != nil {
		return "", fmt.Errorf("parse finished time: %w", err)
	}

	if c.State.Running {
		if c.State.Paused {
			return fmt.Sprintf("Up %s (Paused)", units.HumanDuration(time.Now().UTC().Sub(startedAt))), nil
		}
		if c.State.Restarting {
			return fmt.Sprintf("Restarting (%d) %s ago",
				c.State.ExitCode, units.HumanDuration(time.Now().UTC().Sub(finishedAt))), nil
		}

		if c.State.Health != nil {
			status := c.State.Health.Status
			if status == types.Starting {
				status = "health: " + status
			}

			return fmt.Sprintf("Up %s (%s)", units.HumanDuration(time.Now().UTC().Sub(startedAt)), status), nil
		}

		return fmt.Sprintf("Up %s", units.HumanDuration(time.Now().UTC().Sub(startedAt))), nil
	}

	if c.State.Status == "removing" {
		return "Removal In Progress", nil
	}

	if c.State.Dead {
		return "Dead", nil
	}

	if startedAt.IsZero() {
		return "Created", nil
	}

	if finishedAt.IsZero() {
		return "", nil
	}

	return fmt.Sprintf("Exited (%d) %s ago",
		c.State.ExitCode, units.HumanDuration(time.Now().UTC().Sub(finishedAt))), nil
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
