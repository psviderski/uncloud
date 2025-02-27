package api

import (
	"encoding/json"
	"fmt"
	"github.com/distribution/reference"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"uncloud/internal/machine/api/pb"
)

const (
	ServiceModeReplicated = "replicated"
	ServiceModeGlobal     = "global"
)

type ServiceSpec struct {
	Container ContainerSpec
	// Mode is the replication mode of the service. Default is ServiceModeReplicated if empty.
	Mode string
	Name string
	// Ports defines what service ports to publish to make the service accessible outside the cluster.
	Ports []PortSpec
}

func (s *ServiceSpec) Validate() error {
	if err := s.Container.Validate(); err != nil {
		return err
	}

	switch s.Mode {
	case "", ServiceModeGlobal, ServiceModeReplicated:
	default:
		return fmt.Errorf("invalid mode: %q", s.Mode)
	}

	// TODO: validate there is no conflict between ports.

	return nil
}

func (s *ServiceSpec) Equals(spec ServiceSpec) bool {
	// TODO: ignore order of ports.
	return reflect.DeepEqual(*s, spec)
}

var serviceIDRegexp = regexp.MustCompile("^[0-9a-f]{32}$")

func ValidateServiceID(id string) bool {
	return serviceIDRegexp.MatchString(id)
}

type ContainerSpec struct {
	Command []string
	Image   string
	// Run a custom init inside the container. If nil, use the daemon's configured settings.
	Init *bool
	// List of volumes to bind mount into the container.
	Volumes []string
}

func (s *ContainerSpec) Validate() error {
	if _, err := reference.ParseDockerRef(s.Image); err != nil {
		return fmt.Errorf("invalid image: %w", err)
	}

	return nil
}

type Service struct {
	ID         string
	Name       string
	Mode       string
	Containers []MachineContainer
}

type MachineContainer struct {
	MachineID string
	Container Container
}

// Endpoints returns the exposed HTTP and HTTPS endpoints of the service.
func (s *Service) Endpoints() []string {
	endpoints := make(map[string]struct{})

	// Container specs may differ between containers in the same service, e.g. during a rolling update,
	// so we need to collect all unique endpoints.
	for _, ctr := range s.Containers {
		ports, err := ctr.Container.ServicePorts()
		if err != nil {
			continue
		}

		for _, port := range ports {
			protocol := ""
			switch port.Protocol {
			case ProtocolHTTP:
				protocol = "http"
			case ProtocolHTTPS:
				protocol = "https"
			default:
				continue
			}

			if port.Hostname == "" {
				// There shouldn't be http(s) ports without a hostname but just in case ignore them.
				continue
			}

			endpoint := fmt.Sprintf("%s://%s", protocol, port.Hostname)
			if port.PublishedPort != 0 {
				// For non-standard ports (80/443), include the port in the URL.
				if !(port.Protocol == ProtocolHTTP && port.PublishedPort == 80) &&
					!(port.Protocol == ProtocolHTTPS && port.PublishedPort == 443) {
					endpoint += fmt.Sprintf(":%d", port.PublishedPort)
				}
			}

			endpoints[endpoint] = struct{}{}
		}
	}

	return slices.Sorted(maps.Keys(endpoints))
}

func ServiceFromProto(s *pb.Service) (Service, error) {
	var err error
	containers := make([]MachineContainer, len(s.Containers))
	for i, sc := range s.Containers {
		containers[i], err = machineContainerFromProto(sc)
		if err != nil {
			return Service{}, err
		}
	}

	return Service{
		ID:         s.Id,
		Name:       s.Name,
		Mode:       s.Mode,
		Containers: containers,
	}, nil
}

func machineContainerFromProto(sc *pb.Service_Container) (MachineContainer, error) {
	var c Container
	if err := json.Unmarshal(sc.Container, &c); err != nil {
		return MachineContainer{}, fmt.Errorf("unmarshal container: %w", err)
	}

	return MachineContainer{
		MachineID: sc.MachineId,
		Container: c,
	}, nil
}
