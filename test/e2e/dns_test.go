package e2e

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInternalDNS tests the internal DNS functionality including the new machine-specific service lookups
func TestInternalDNS(t *testing.T) {
	t.Parallel()

	clusterName := "ucind-test.dns"
	ctx := context.Background()
	c, _ := createTestCluster(t, clusterName, ucind.CreateClusterOptions{Machines: 3}, true)

	cli, err := c.Machines[0].Connect(ctx)
	require.NoError(t, err)

	// Create a test service with multiple replicas across machines
	serviceName := "test-dns-service"
	t.Cleanup(func() {
		err := cli.RemoveService(ctx, serviceName)
		if err != nil && !strings.Contains(err.Error(), "not found") {
			require.NoError(t, err)
		}
	})

	// Deploy a service across all machines in global mode using pause container
	spec := api.ServiceSpec{
		Name: serviceName,
		Mode: api.ServiceModeGlobal,
		Container: api.ContainerSpec{
			Image: "portainer/pause:latest",
		},
	}

	deployment := cli.NewDeployment(spec, nil)
	_, err = deployment.Run(ctx)
	require.NoError(t, err)

	// Wait for the service to be deployed
	var svc api.Service
	require.Eventually(t, func() bool {
		svc, err = cli.InspectService(ctx, serviceName)
		if err != nil {
			return false
		}
		// Should have 3 containers (one per machine) and all should be running
		if len(svc.Containers) != 3 {
			return false
		}
		for _, ctr := range svc.Containers {
			if ctr.Container.State.Status != "running" {
				return false
			}
		}
		return true
	}, 30*time.Second, 1*time.Second, "Service should be deployed and running on all machines")

	// Deploy a single "network-multitool" service to be used for all DNS queries
	queryServiceName := "dns-query-service"
	t.Cleanup(func() {
		err := cli.RemoveService(ctx, queryServiceName)
		if err != nil && !strings.Contains(err.Error(), "not found") {
			require.NoError(t, err)
		}
	})
	querySvcSpec := api.ServiceSpec{
		Name:     queryServiceName,
		Mode:     api.ServiceModeReplicated,
		Replicas: 1,
		Placement: api.Placement{
			Machines: []string{c.Machines[0].Name},
		},
		Container: api.ContainerSpec{
			Image:   "wbitt/network-multitool",
			Command: []string{"sleep", "infinity"},
		},
	}
	_, err = cli.RunService(ctx, querySvcSpec)
	require.NoError(t, err)

	// Wait for the query service to be deployed
	var querySvc api.Service
	require.Eventually(t, func() bool {
		querySvc, err = cli.InspectService(ctx, queryServiceName)
		if err != nil {
			return false
		}
		// Should have 1 container and it should be running
		if len(querySvc.Containers) != 1 {
			return false
		}
		return querySvc.Containers[0].Container.State.Status == "running"
	}, 30*time.Second, 1*time.Second, "Query service should be deployed and running")
	queryContainer := querySvc.Containers[0]

	// Run nslookup for given query
	runNslookup := func(t *testing.T, dnsQuery string) string {
		dnsOutput, err := execInContainerAndReadOutput(
			t, ctx, cli, queryServiceName, queryContainer.Container.ID,
			[]string{"nslookup", dnsQuery},
		)
		require.NoError(t, err)
		return dnsOutput
	}

	// Helper function to verify DNS output doesn't contain errors
	assertNoDNSErrors := func(t *testing.T, dnsOutput string) {
		assert.NotContains(t, dnsOutput, "can't resolve", "DNS query should not contain resolution errors")
		assert.NotContains(t, dnsOutput, "Name or service not known",
			"DNS query should not contain unknown service errors")
	}

	t.Run("service name resolves to all container IPs", func(t *testing.T) {
		dnsOutput := runNslookup(t, serviceName+".internal")
		t.Logf("DNS query output:\n%s", dnsOutput)

		// Verify that all service container IPs are in the DNS response
		for _, ctr := range svc.Containers {
			containerIP := ctr.Container.UncloudNetworkIP().String()
			assert.Contains(t, dnsOutput, containerIP,
				"Service DNS should resolve to container IP %s", containerIP)
		}

		assertNoDNSErrors(t, dnsOutput)
	})

	t.Run("machine-specific service DNS lookups", func(t *testing.T) {
		// Test the new <machine-id>.m.<service-name>.internal DNS feature
		for _, targetContainer := range svc.Containers {
			targetMachineID := targetContainer.MachineID
			targetContainerIP := targetContainer.Container.UncloudNetworkIP().String()

			// Construct the machine-specific DNS name
			machineSpecificDNS := targetMachineID + ".m." + serviceName + ".internal"

			dnsOutput := runNslookup(t, machineSpecificDNS)
			t.Logf("Machine-specific DNS query output for %s:\n%s", machineSpecificDNS, dnsOutput)

			// Verify that the specific container IP is returned
			assert.Contains(t, dnsOutput, targetContainerIP,
				"Machine-specific DNS %s should resolve to container IP %s",
				machineSpecificDNS, targetContainerIP)

			// Verify that other container IPs are not returned (machine-specific should return only one IP)
			for _, ctr := range svc.Containers {
				if ctr.MachineID != targetMachineID {
					otherContainerIP := ctr.Container.UncloudNetworkIP().String()
					assert.NotContains(t, dnsOutput, otherContainerIP,
						"Machine-specific DNS %s should not resolve to other container IP %s",
						machineSpecificDNS, otherContainerIP)
				}
			}

			assertNoDNSErrors(t, dnsOutput)
		}
	})

	t.Run("service ID DNS lookup", func(t *testing.T) {
		// Test that service ID also resolves (existing functionality)
		dnsOutput := runNslookup(t, svc.ID+".internal")
		t.Logf("Service ID DNS query output:\n%s", dnsOutput)

		// Verify that all service container IPs are in the DNS response
		for _, ctr := range svc.Containers {
			containerIP := ctr.Container.UncloudNetworkIP().String()
			assert.Contains(t, dnsOutput, containerIP,
				"Service ID DNS should resolve to container IP %s", containerIP)
		}

		assertNoDNSErrors(t, dnsOutput)
	})

	t.Run("nearest mode prioritizes local subnet IPs", func(t *testing.T) {
		// Find the service container on the same machine as the query container.
		var localIP string
		for _, ctr := range svc.Containers {
			if ctr.MachineID == queryContainer.MachineID {
				localIP = ctr.Container.UncloudNetworkIP().String()
				break
			}
		}
		require.NotEmpty(t, localIP, "Should find local container IP on query machine %s", queryContainer.MachineID)

		// We will extract the first IP address from the DNS output using a regex.
		// Pattern matches "Name: nearest.test-dns-service.internal" followed by "Address: X.X.X.X".
		re := regexp.MustCompile(`(?m)Name:\s+[\w\.\-]+\s+Address:\s+([\d\.]+)`)

		// Test the "nearest" mode which should sort local subnet IPs first.
		// The default behavior randomizes the order, so run it a few times
		// to reduce the chance we're just getting lucky with the order.
		for range 5 {
			dnsOutput := runNslookup(t, "nearest."+serviceName+".internal")
			t.Logf("Nearest mode DNS query output:\n%s", dnsOutput)

			matches := re.FindStringSubmatch(dnsOutput)
			require.Len(t, matches, 2, "Should find Name's Address in DNS output")
			firstIP := matches[1]
			assert.Equal(t, localIP, firstIP,
				"Nearest mode should return local subnet IP first (query machine: %s, local IP: %s, first DNS result: %s)",
				queryContainer.MachineID, localIP, firstIP)

			assertNoDNSErrors(t, dnsOutput)
		}
	})
}
