package deploy

import (
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceSpecResolver_generateHostname(t *testing.T) {
	tests := []struct {
		name          string
		serviceName   string
		namespace     string
		clusterDomain string
		expected      string
	}{
		{
			name:          "default namespace",
			serviceName:   "web",
			namespace:     api.DefaultNamespace,
			clusterDomain: "abc123.cluster.uncloud.run",
			expected:      "web.abc123.cluster.uncloud.run",
		},
		{
			name:          "empty namespace treated as default",
			serviceName:   "web",
			namespace:     "",
			clusterDomain: "abc123.cluster.uncloud.run",
			expected:      "web.abc123.cluster.uncloud.run",
		},
		{
			name:          "prod namespace",
			serviceName:   "api",
			namespace:     "prod",
			clusterDomain: "abc123.cluster.uncloud.run",
			expected:      "api-prod.abc123.cluster.uncloud.run",
		},
		{
			name:          "system namespace",
			serviceName:   "caddy",
			namespace:     api.SystemNamespace,
			clusterDomain: "abc123.cluster.uncloud.run",
			expected:      "caddy-uncloud-system.abc123.cluster.uncloud.run",
		},
		{
			name:          "custom namespace with hyphens",
			serviceName:   "service",
			namespace:     "my-namespace",
			clusterDomain: "abc123.cluster.uncloud.run",
			expected:      "service-my-namespace.abc123.cluster.uncloud.run",
		},
		{
			name:          "staging namespace",
			serviceName:   "backend",
			namespace:     "staging",
			clusterDomain: "xyz789.cluster.uncloud.run",
			expected:      "backend-staging.xyz789.cluster.uncloud.run",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &ServiceSpecResolver{
				ClusterDomain: tt.clusterDomain,
			}

			result := resolver.generateHostname(tt.serviceName, tt.namespace)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestServiceSpecResolver_expandIngressPorts_NamespaceAware(t *testing.T) {
	tests := []struct {
		name              string
		serviceName       string
		namespace         string
		clusterDomain     string
		initialPorts      []api.PortSpec
		expectedHostnames []string
		expectError       bool
	}{
		{
			name:          "default namespace - namespace omitted from hostname",
			serviceName:   "web",
			namespace:     api.DefaultNamespace,
			clusterDomain: "abc123.cluster.uncloud.run",
			initialPorts: []api.PortSpec{
				{ContainerPort: 80, Protocol: api.ProtocolHTTP},
			},
			expectedHostnames: []string{"web.abc123.cluster.uncloud.run"},
		},
		{
			name:          "empty namespace treated as default",
			serviceName:   "web",
			namespace:     "",
			clusterDomain: "abc123.cluster.uncloud.run",
			initialPorts: []api.PortSpec{
				{ContainerPort: 80, Protocol: api.ProtocolHTTP},
			},
			expectedHostnames: []string{"web.abc123.cluster.uncloud.run"},
		},
		{
			name:          "prod namespace - namespace included in hostname",
			serviceName:   "web",
			namespace:     "prod",
			clusterDomain: "abc123.cluster.uncloud.run",
			initialPorts: []api.PortSpec{
				{ContainerPort: 80, Protocol: api.ProtocolHTTP},
			},
			expectedHostnames: []string{"web-prod.abc123.cluster.uncloud.run"},
		},
		{
			name:          "system namespace - namespace included in hostname",
			serviceName:   "caddy",
			namespace:     api.SystemNamespace,
			clusterDomain: "abc123.cluster.uncloud.run",
			initialPorts: []api.PortSpec{
				{ContainerPort: 80, Protocol: api.ProtocolHTTP},
			},
			expectedHostnames: []string{"caddy-uncloud-system.abc123.cluster.uncloud.run"},
		},
		{
			name:          "custom namespace - namespace included in hostname",
			serviceName:   "api",
			namespace:     "staging",
			clusterDomain: "abc123.cluster.uncloud.run",
			initialPorts: []api.PortSpec{
				{ContainerPort: 8000, Protocol: api.ProtocolHTTPS},
			},
			expectedHostnames: []string{"api-staging.abc123.cluster.uncloud.run"},
		},
		{
			name:          "external domain - duplicates with namespace-aware cluster domain",
			serviceName:   "web",
			namespace:     "prod",
			clusterDomain: "abc123.cluster.uncloud.run",
			initialPorts: []api.PortSpec{
				{ContainerPort: 80, Protocol: api.ProtocolHTTP, Hostname: "example.com"},
			},
			expectedHostnames: []string{"example.com", "web-prod.abc123.cluster.uncloud.run"},
		},
		{
			name:          "external domain in default namespace - no namespace in duplicate",
			serviceName:   "web",
			namespace:     api.DefaultNamespace,
			clusterDomain: "abc123.cluster.uncloud.run",
			initialPorts: []api.PortSpec{
				{ContainerPort: 80, Protocol: api.ProtocolHTTP, Hostname: "example.com"},
			},
			expectedHostnames: []string{"example.com", "web.abc123.cluster.uncloud.run"},
		},
		{
			name:          "user-provided cluster subdomain - not duplicated",
			serviceName:   "web",
			namespace:     "prod",
			clusterDomain: "abc123.cluster.uncloud.run",
			initialPorts: []api.PortSpec{
				{ContainerPort: 80, Protocol: api.ProtocolHTTP, Hostname: "custom.abc123.cluster.uncloud.run"},
			},
			expectedHostnames: []string{"custom.abc123.cluster.uncloud.run"},
		},
		{
			name:          "multiple ports - all get namespace-aware hostnames",
			serviceName:   "api",
			namespace:     "staging",
			clusterDomain: "abc123.cluster.uncloud.run",
			initialPorts: []api.PortSpec{
				{ContainerPort: 80, Protocol: api.ProtocolHTTP},
				{ContainerPort: 443, Protocol: api.ProtocolHTTPS},
			},
			expectedHostnames: []string{
				"api-staging.abc123.cluster.uncloud.run",
				"api-staging.abc123.cluster.uncloud.run",
			},
		},
		{
			name:          "multiple http/https ports",
			serviceName:   "app",
			namespace:     "prod",
			clusterDomain: "abc123.cluster.uncloud.run",
			initialPorts: []api.PortSpec{
				{ContainerPort: 80, Protocol: api.ProtocolHTTP},
				{ContainerPort: 443, Protocol: api.ProtocolHTTPS},
			},
			expectedHostnames: []string{
				"app-prod.abc123.cluster.uncloud.run",
				"app-prod.abc123.cluster.uncloud.run",
			},
		},
		{
			name:          "no cluster domain - error expected",
			serviceName:   "web",
			namespace:     "prod",
			clusterDomain: "",
			initialPorts: []api.PortSpec{
				{ContainerPort: 80, Protocol: api.ProtocolHTTP},
			},
			expectError: true,
		},
		{
			name:          "external domain without cluster domain - uses only external",
			serviceName:   "web",
			namespace:     "prod",
			clusterDomain: "",
			initialPorts: []api.PortSpec{
				{ContainerPort: 80, Protocol: api.ProtocolHTTP, Hostname: "example.com"},
			},
			expectedHostnames: []string{"example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &ServiceSpecResolver{
				ClusterDomain: tt.clusterDomain,
			}

			spec := api.ServiceSpec{
				Name:      tt.serviceName,
				Namespace: tt.namespace,
				Container: api.ContainerSpec{
					Image: "nginx:latest",
				},
				Ports: tt.initialPorts,
			}

			resolved, err := resolver.Resolve(spec)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			var actualHostnames []string
			for _, port := range resolved.Ports {
				if port.Hostname != "" {
					actualHostnames = append(actualHostnames, port.Hostname)
				}
			}

			assert.Equal(t, tt.expectedHostnames, actualHostnames)
		})
	}
}

func TestServiceSpecResolver_Resolve_NamespaceDefaults(t *testing.T) {
	resolver := &ServiceSpecResolver{
		ClusterDomain: "abc123.cluster.uncloud.run",
	}

	// Test that namespace defaults to "default" when not provided
	spec := api.ServiceSpec{
		Name: "web",
		// Namespace not specified
		Container: api.ContainerSpec{
			Image: "nginx:latest",
		},
		Ports: []api.PortSpec{
			{ContainerPort: 80, Protocol: api.ProtocolHTTP},
		},
	}

	resolved, err := resolver.Resolve(spec)
	require.NoError(t, err)

	// Verify namespace was set to default
	assert.Equal(t, api.DefaultNamespace, resolved.Namespace)

	// Verify hostname doesn't include namespace
	require.Len(t, resolved.Ports, 1)
	assert.Equal(t, "web.abc123.cluster.uncloud.run", resolved.Ports[0].Hostname)
}
