package api

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"

	"github.com/distribution/reference"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

const (
	ServiceModeReplicated = "replicated"
	ServiceModeGlobal     = "global"

	// PullPolicyAlways means the image is always pulled from the registry.
	PullPolicyAlways = "always"
	// PullPolicyMissing means the image is pulled from the registry only if it's not available on the machine where
	// a container is started. This is the default pull policy.
	// TODO: make each machine aware of the images on other machines and it possible to pull from them.
	// 	Pull from the registry only if the image is missing on all machines.
	PullPolicyMissing = "missing"
	// PullPolicyNever means the image is never pulled from the registry. A service with this pull policy can only be
	// deployed to machines where the image is already available.
	// TODO: see the TODO above for PullPolicyMissing. Pull from other machines in the cluster if available.
	PullPolicyNever = "never"
)

var serviceIDRegexp = regexp.MustCompile("^[0-9a-f]{32}$")

func ValidateServiceID(id string) bool {
	return serviceIDRegexp.MatchString(id)
}

// ServiceSpec defines the desired state of a service.
// ATTENTION: after changing this struct, verify if deploy.EvalContainerSpecChange needs to be updated.
type ServiceSpec struct {
	Container ContainerSpec
	// Mode is the replication mode of the service. Default is ServiceModeReplicated if empty.
	Mode string
	Name string
	// Ports defines what service ports to publish to make the service accessible outside the cluster.
	Ports []PortSpec
	// Replicas is the number of containers to run for the service. Only valid for a replicated service.
	Replicas uint `json:",omitempty"`
}

func (s *ServiceSpec) SetDefaults() ServiceSpec {
	spec := s.Clone()

	if spec.Mode == "" {
		spec.Mode = ServiceModeReplicated
	}
	// Ensure the replicated service has at least one replica.
	if spec.Mode == ServiceModeReplicated && spec.Replicas == 0 {
		spec.Replicas = 1
	}
	spec.Container = spec.Container.SetDefaults()

	return spec
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

	// TODO: validate the service name is a valid DNS label.

	for _, p := range s.Ports {
		if (p.Mode == "" || p.Mode == PortModeIngress) &&
			p.Protocol != ProtocolHTTP && p.Protocol != ProtocolHTTPS {
			return fmt.Errorf("unsupported protocol for ingress port %d: %s", p.ContainerPort, p.Protocol)
		}
	}

	// TODO: validate there is no conflict between ports.

	return nil
}

func (s *ServiceSpec) Clone() ServiceSpec {
	spec := *s

	if s.Ports != nil {
		spec.Ports = make([]PortSpec, len(s.Ports))
		copy(spec.Ports, s.Ports)
	}
	spec.Container = s.Container.Clone()

	return spec
}

// ContainerSpec defines the desired state of a container in a service.
// ATTENTION: after changing this struct, verify if deploy.EvalContainerSpecChange needs to be updated.
type ContainerSpec struct {
	// Command overrides the default CMD of the image to be executed when running a container.
	Command []string
	// Entrypoint overrides the default ENTRYPOINT of the image.
	Entrypoint []string
	Image      string
	// Run a custom init inside the container. If nil, use the daemon's configured settings.
	Init *bool
	// PullPolicy determines when to pull the image from the registry or use the image already available in the cluster.
	// Default is PullPolicyMissing if empty.
	PullPolicy string
	// List of volumes to bind mount into the container.
	Volumes []string
}

// SetDefaults returns a copy of the container spec with default values set.
func (s *ContainerSpec) SetDefaults() ContainerSpec {
	spec := s.Clone()
	if spec.PullPolicy == "" {
		spec.PullPolicy = PullPolicyMissing
	}

	return spec
}

func (s *ContainerSpec) Validate() error {
	if _, err := reference.ParseDockerRef(s.Image); err != nil {
		return fmt.Errorf("invalid image: %w", err)
	}

	return nil
}

func (s *ContainerSpec) Equals(spec ContainerSpec) bool {
	orig := s.SetDefaults()
	spec = spec.SetDefaults()

	slices.Sort(orig.Volumes)
	slices.Sort(spec.Volumes)

	return reflect.DeepEqual(orig, spec)
}

func (s *ContainerSpec) Clone() ContainerSpec {
	spec := *s

	if s.Command != nil {
		spec.Command = make([]string, len(s.Command))
		copy(spec.Command, s.Command)
	}
	if s.Entrypoint != nil {
		spec.Entrypoint = make([]string, len(s.Entrypoint))
		copy(spec.Entrypoint, s.Entrypoint)
	}
	if s.Volumes != nil {
		spec.Volumes = make([]string, len(s.Volumes))
		copy(spec.Volumes, s.Volumes)
	}

	return spec
}

type Service struct {
	ID         string
	Name       string
	Mode       string
	Containers []MachineServiceContainer
}

type MachineServiceContainer struct {
	MachineID string
	Container ServiceContainer
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

			endpoint += fmt.Sprintf(" → :%d", port.ContainerPort)
			endpoints[endpoint] = struct{}{}
		}
	}

	return slices.Sorted(maps.Keys(endpoints))
}

func ServiceFromProto(s *pb.Service) (Service, error) {
	var err error
	containers := make([]MachineServiceContainer, len(s.Containers))
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

func machineContainerFromProto(sc *pb.Service_Container) (MachineServiceContainer, error) {
	var c Container
	if err := json.Unmarshal(sc.Container, &c); err != nil {
		return MachineServiceContainer{}, fmt.Errorf("unmarshal container: %w", err)
	}

	return MachineServiceContainer{
		MachineID: sc.MachineId,
		Container: ServiceContainer{Container: c},
	}, nil
}
