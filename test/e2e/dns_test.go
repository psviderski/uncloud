package e2e

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
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

	// Get Docker client to access ucind machine containers (shared across all DNS tests)
	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	// Helper function to create DNS query service spec
	createDNSQuerySpec := func(name, dnsQuery, outputFile string) api.ServiceSpec {
		return api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Image: "wbitt/network-multitool",
				Command: []string{"sh", "-c", fmt.Sprintf("nslookup %s > %s 2>&1 && echo 'DNS query completed' && sleep infinity",
					dnsQuery, outputFile)},
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    "host-tmp",
						ContainerPath: "/tmp",
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: "host-tmp",
					Type: "bind",
					BindOptions: &api.BindOptions{
						HostPath: "/tmp",
					},
				},
			},
			Replicas: 1,
		}
	}

	// Helper function to run DNS query and get results
	runDNSQuery := func(t *testing.T, serviceName, dnsQuery, outputFile string) string {
		spec := createDNSQuerySpec(serviceName, dnsQuery, outputFile)

		t.Cleanup(func() {
			cli.RemoveService(ctx, spec.Name)
		})

		// Run the DNS query service
		resp, err := cli.RunService(ctx, spec)
		require.NoError(t, err)

		// Wait for the DNS query to complete
		var querySvc api.Service
		require.Eventually(t, func() bool {
			querySvc, err = cli.InspectService(ctx, resp.ID)
			if err != nil || len(querySvc.Containers) == 0 {
				return false
			}
			return querySvc.Containers[0].Container.State.Status == "running"
		}, 30*time.Second, 1*time.Second, "DNS query container should be running")

		// Give some time for DNS query to complete and write to file
		time.Sleep(1 * time.Second)

		// Find which ucind machine the DNS query container is running on
		ctr := querySvc.Containers[0]
		machineID := ctr.MachineID

		// Get the ucind machine container name for this machine
		var ucindMachineName string
		for _, machine := range c.Machines {
			if machine.ID == machineID {
				ucindMachineName = machine.ContainerName
				break
			}
		}
		require.NotEmpty(t, ucindMachineName, "Should find ucind machine container name")

		// Read the DNS results from the ucind machine's filesystem
		exec, err := dockerCli.ContainerExecCreate(ctx, ucindMachineName, container.ExecOptions{
			Cmd:          []string{"cat", outputFile},
			AttachStdout: true,
			AttachStderr: true,
		})
		require.NoError(t, err)

		resp2, err := dockerCli.ContainerExecAttach(ctx, exec.ID, container.ExecAttachOptions{})
		require.NoError(t, err)
		defer resp2.Close()

		// Read the DNS output
		buf := make([]byte, 4096)
		n, _ := resp2.Reader.Read(buf)
		return string(buf[:n])
	}

	// Helper function to verify DNS output doesn't contain errors
	assertNoDNSErrors := func(t *testing.T, dnsOutput string) {
		assert.NotContains(t, dnsOutput, "can't resolve", "DNS query should not contain resolution errors")
		assert.NotContains(t, dnsOutput, "Name or service not known",
			"DNS query should not contain unknown service errors")
	}

	t.Run("service name resolves to all container IPs", func(t *testing.T) {
		dnsOutput := runDNSQuery(t, "dns-test-service-name", serviceName+".internal", "/tmp/dns_result.txt")
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
		for i, targetContainer := range svc.Containers {
			targetMachineID := targetContainer.MachineID
			targetContainerIP := targetContainer.Container.UncloudNetworkIP().String()

			// Construct the machine-specific DNS name
			machineSpecificDNS := targetMachineID + ".m." + serviceName + ".internal"
			outputFile := fmt.Sprintf("/tmp/dns_result_machine_%d.txt", i)

			dnsOutput := runDNSQuery(t, fmt.Sprintf("dns-test-machine-%d", i), machineSpecificDNS, outputFile)
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
		dnsOutput := runDNSQuery(t, "dns-test-service-id", svc.ID+".internal", "/tmp/dns_result_service_id.txt")
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
		// Test the "nearest" mode which should sort local subnet IPs first
		dnsOutput := runDNSQuery(t, "dns-test-nearest", "nearest."+serviceName+".internal", "/tmp/dns_result_nearest.txt")
		t.Logf("Nearest mode DNS query output:\n%s", dnsOutput)

		// Find which machine ran the query
		querySvc, err := cli.InspectService(ctx, "dns-test-nearest")
		require.NoError(t, err)
		require.NotEmpty(t, querySvc.Containers, "Query service should have a container")
		queryMachineID := querySvc.Containers[0].MachineID

		// Find the local container IP (on the same machine as the query)
		var localIP string
		for _, ctr := range svc.Containers {
			if ctr.MachineID == queryMachineID {
				localIP = ctr.Container.UncloudNetworkIP().String()
				break
			}
		}
		require.NotEmpty(t, localIP, "Should find local container IP on query machine %s", queryMachineID)

		// Extract the first IP address from the DNS output using regex
		// Pattern matches: "Name: nearest.test-dns-service.internal" followed by "Address: X.X.X.X"
		re := regexp.MustCompile(`(?m)Name:\s+[\w\.\-]+\s+Address:\s+([\d\.]+)`)
		matches := re.FindStringSubmatch(dnsOutput)
		require.Len(t, matches, 2, "Should find Name's Address in DNS output")
		firstIP := matches[1]
		assert.Equal(t, localIP, firstIP,
			"Nearest mode should return local subnet IP first (query machine: %s, local IP: %s, first DNS result: %s)",
			queryMachineID, localIP, firstIP)

		assertNoDNSErrors(t, dnsOutput)
	})
}
