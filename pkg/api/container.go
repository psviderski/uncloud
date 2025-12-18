package api

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/go-units"
)

const (
	// DockerNetworkName is the name of the Docker network used by uncloud. Keep the value in sync with NetworkName
	// in internal/machine/docker/manager.go.
	DockerNetworkName = "uncloud"
	LabelManaged      = "uncloud.managed"
	LabelServiceID    = "uncloud.service.id"
	LabelServiceName  = "uncloud.service.name"
	LabelServiceMode  = "uncloud.service.mode"
	LabelServicePorts = "uncloud.service.ports"
)

type Container struct {
	container.InspectResponse
	// created caches the parsed creation time by CreatedTime.
	created time.Time
}

// CreatedTime returns the time when the container was created parsed from the Created field.
func (c *Container) CreatedTime() time.Time {
	if c.created.IsZero() && c.Created != "" {
		created, err := time.Parse(time.RFC3339Nano, c.Created)
		if err != nil {
			return time.Time{}
		}
		c.created = created
	}
	return c.created
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

	return c.State.Health.Status == container.Healthy
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
			if status == container.Starting {
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

// UncloudNetworkIP returns the IP address of the container in the uncloud Docker network.
func (c *Container) UncloudNetworkIP() netip.Addr {
	network, ok := c.NetworkSettings.Networks[DockerNetworkName]
	if !ok {
		// Container is not connected to the uncloud Docker network (could be host network).
		return netip.Addr{}
	}

	ip, err := netip.ParseAddr(network.IPAddress)
	if err != nil {
		return netip.Addr{}
	}

	return ip
}

func (c *Container) UnmarshalJSON(data []byte) error {
	// A temporary type that's identical to Container but doesn't have the UnmarshalJSON method.
	type ContainerAlias Container

	var temp ContainerAlias
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	*c = Container(temp)
	if c.ContainerJSONBase == nil {
		return fmt.Errorf("container data is missing mandatory base fields: %s", data)
	}

	c.Name = strings.TrimPrefix(c.Name, "/")

	return nil
}

type ServiceContainer struct {
	Container
	ServiceSpec ServiceSpec
}

// ShortID returns the truncated ID of the container (12 characters).
func (c *ServiceContainer) ShortID() string {
	return stringid.TruncateID(c.ID)
}

// ServiceID returns the ID of the service this container belongs to.
func (c *ServiceContainer) ServiceID() string {
	return c.Config.Labels[LabelServiceID]
}

// ServiceName returns the name of the service this container belongs to.
func (c *ServiceContainer) ServiceName() string {
	return c.Config.Labels[LabelServiceName]
}

// ServiceMode returns the replication mode of the service this container belongs to.
func (c *ServiceContainer) ServiceMode() string {
	return c.Config.Labels[LabelServiceMode]
}

// ServicePorts returns the ports this container publishes as part of its service.
func (c *ServiceContainer) ServicePorts() ([]PortSpec, error) {
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

// ConflictingServicePorts returns a list of service ports that conflict with the given ports.
func (c *ServiceContainer) ConflictingServicePorts(ports []PortSpec) ([]PortSpec, error) {
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

// UnmarshalJSON implements custom unmarshalling for ServiceContainer to override the custom unmarshaler
// of the embedded Container field.
func (c *ServiceContainer) UnmarshalJSON(data []byte) error {
	// Unmarshal everything except Container into a temporary struct. Keep this in sync with ServiceContainer.
	var temp struct {
		ServiceSpec ServiceSpec
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Let Container's UnmarshalJSON handle its part.
	if err := json.Unmarshal(data, &c.Container); err != nil {
		return err
	}

	c.ServiceSpec = temp.ServiceSpec

	return nil
}
