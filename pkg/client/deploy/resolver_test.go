package deploy

import (
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
)

func TestServiceSpecResolver_AllocateTCPPorts(t *testing.T) {
	tests := []struct {
		name        string
		spec        api.ServiceSpec
		usedPorts   map[uint16]struct{}
		wantPortSet bool // true if we expect a port to be allocated
		wantErr     bool
		wantInRange bool // true if the allocated port should be in the TCP range
	}{
		{
			name: "allocate port for TCP ingress without published port",
			spec: api.ServiceSpec{
				Name: "test-svc",
				Container: api.ContainerSpec{
					Image: "nginx:latest",
				},
				Ports: []api.PortSpec{
					{ContainerPort: 5432, Protocol: api.ProtocolTCP, Mode: api.PortModeIngress},
				},
			},
			wantPortSet: true,
			wantInRange: true,
		},
		{
			name: "keep user-specified published port",
			spec: api.ServiceSpec{
				Name: "test-svc",
				Container: api.ContainerSpec{
					Image: "nginx:latest",
				},
				Ports: []api.PortSpec{
					{ContainerPort: 5432, PublishedPort: 35432, Protocol: api.ProtocolTCP, Mode: api.PortModeIngress},
				},
			},
			wantPortSet: true,
			wantInRange: false, // 35432 is in range but was user-specified
		},
		{
			name: "skip host mode TCP port",
			spec: api.ServiceSpec{
				Name: "test-svc",
				Container: api.ContainerSpec{
					Image: "nginx:latest",
				},
				Ports: []api.PortSpec{
					{ContainerPort: 5432, Protocol: api.ProtocolTCP, Mode: api.PortModeHost},
				},
			},
			wantPortSet: false,
		},
		{
			name: "skip HTTP port",
			spec: api.ServiceSpec{
				Name: "test-svc",
				Container: api.ContainerSpec{
					Image: "nginx:latest",
				},
				Ports: []api.PortSpec{
					{ContainerPort: 8080, Protocol: api.ProtocolHTTP, Mode: api.PortModeIngress},
				},
			},
			wantPortSet: false,
		},
		{
			name: "avoid used ports",
			spec: api.ServiceSpec{
				Name: "test-svc",
				Container: api.ContainerSpec{
					Image: "nginx:latest",
				},
				Ports: []api.PortSpec{
					{ContainerPort: 5432, Protocol: api.ProtocolTCP, Mode: api.PortModeIngress},
				},
			},
			usedPorts: map[uint16]struct{}{
				30000: {},
				30001: {},
				30002: {},
			},
			wantPortSet: true,
			wantInRange: true,
		},
		{
			name: "default mode is ingress",
			spec: api.ServiceSpec{
				Name: "test-svc",
				Container: api.ContainerSpec{
					Image: "nginx:latest",
				},
				Ports: []api.PortSpec{
					{ContainerPort: 5432, Protocol: api.ProtocolTCP}, // Mode empty = ingress
				},
			},
			wantPortSet: true,
			wantInRange: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &ServiceSpecResolver{
				ClusterDomain: "test.uncloud.run",
				UsedTCPPorts:  tt.usedPorts,
			}

			resolved, err := resolver.Resolve(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(resolved.Ports) == 0 {
				if tt.wantPortSet {
					t.Error("Expected ports to be set")
				}
				return
			}

			port := resolved.Ports[0]
			if tt.wantPortSet {
				if port.PublishedPort == 0 {
					t.Error("Expected PublishedPort to be set, got 0")
				}
				if tt.wantInRange {
					if port.PublishedPort < TCPPortRangeMin || port.PublishedPort > TCPPortRangeMax {
						t.Errorf("Expected port in range %d-%d, got %d",
							TCPPortRangeMin, TCPPortRangeMax, port.PublishedPort)
					}
					// Verify the port is now tracked as used
					if _, ok := resolver.UsedTCPPorts[port.PublishedPort]; !ok {
						t.Error("Expected allocated port to be tracked in UsedTCPPorts")
					}
				}
			} else {
				if port.Protocol == api.ProtocolTCP && port.PublishedPort != 0 && port.PublishedPort != tt.spec.Ports[0].PublishedPort {
					t.Errorf("Unexpected port allocation: got %d", port.PublishedPort)
				}
			}
		})
	}
}

func TestServiceSpecResolver_AllocateTCPPorts_AvoidConflicts(t *testing.T) {
	// Test that multiple services get different ports
	resolver := &ServiceSpecResolver{
		ClusterDomain: "test.uncloud.run",
		UsedTCPPorts:  make(map[uint16]struct{}),
	}

	allocatedPorts := make(map[uint16]bool)

	for i := 0; i < 10; i++ {
		spec := api.ServiceSpec{
			Name: "test-svc",
			Container: api.ContainerSpec{
				Image: "postgres:latest",
			},
			Ports: []api.PortSpec{
				{ContainerPort: 5432, Protocol: api.ProtocolTCP, Mode: api.PortModeIngress},
			},
		}

		resolved, err := resolver.Resolve(spec)
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}

		port := resolved.Ports[0].PublishedPort
		if allocatedPorts[port] {
			t.Errorf("Port %d was allocated twice", port)
		}
		allocatedPorts[port] = true
	}

	// Verify all 10 different ports were allocated
	if len(allocatedPorts) != 10 {
		t.Errorf("Expected 10 unique ports, got %d", len(allocatedPorts))
	}
}

func TestServiceSpecResolver_AllocateTCPPorts_MultiplePortsInSpec(t *testing.T) {
	resolver := &ServiceSpecResolver{
		ClusterDomain: "test.uncloud.run",
		UsedTCPPorts:  make(map[uint16]struct{}),
	}

	spec := api.ServiceSpec{
		Name: "multi-port-svc",
		Container: api.ContainerSpec{
			Image: "multi:latest",
		},
		Ports: []api.PortSpec{
			{ContainerPort: 5432, Protocol: api.ProtocolTCP, Mode: api.PortModeIngress},
			{ContainerPort: 6379, Protocol: api.ProtocolTCP, Mode: api.PortModeIngress},
			{ContainerPort: 3306, Protocol: api.ProtocolTCP, Mode: api.PortModeIngress},
		},
	}

	resolved, err := resolver.Resolve(spec)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	// Verify all ports got different published ports
	allocatedPorts := make(map[uint16]bool)
	for _, port := range resolved.Ports {
		if port.Protocol != api.ProtocolTCP {
			continue
		}
		if port.PublishedPort == 0 {
			t.Error("TCP ingress port should have PublishedPort set")
		}
		if allocatedPorts[port.PublishedPort] {
			t.Errorf("Port %d was allocated twice within same spec", port.PublishedPort)
		}
		allocatedPorts[port.PublishedPort] = true
	}

	if len(allocatedPorts) != 3 {
		t.Errorf("Expected 3 unique ports, got %d", len(allocatedPorts))
	}
}
