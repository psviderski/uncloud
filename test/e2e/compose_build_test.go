package e2e

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"testing"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/psviderski/uncloud/internal/cli"
	cliInternal "github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeBuild(t *testing.T) {
	t.Parallel()

	clusterName := "ucind-test.compose-build"
	ctx := context.Background()

	registryInternalPort := "5001/tcp"
	clusterOpts := ucind.CreateClusterOptions{
		Machines: 1,
		PortMap: nat.PortMap{
			// Forwarding the registry port to the host.
			nat.Port(registryInternalPort): []nat.PortBinding{
				{
					HostIP: "127.0.0.1",
					// Explicitly specify the mapped host port for the registry because if not specified,
					// 'docker push' from Docker Desktop is unable to reach the randomly mapped one for some reason.
					HostPort: "50000",
				},
			},
		},
	}
	c, _ := createTestCluster(t, clusterName, clusterOpts, true)

	machine := c.Machines[0]
	client, err := machine.Connect(ctx)
	require.NoError(t, err)

	// Get the (random) assigned host port for the future registry service
	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)
	containerInfo, err := dockerCli.ContainerInspect(ctx, machine.ContainerName)
	require.NoError(t, err)
	portMap := containerInfo.NetworkSettings.Ports
	registryHostPortStr := portMap[nat.Port(registryInternalPort)][0].HostPort
	registryHostPort, err := strconv.Atoi(registryHostPortStr)
	require.NoError(t, err)
	assert.True(t, registryHostPort > 0, "Expected host port to be a valid number")

	t.Run("build and push images for a basic project", func(t *testing.T) {
		t.Parallel()

		registryServiceName := "registry"
		t.Cleanup(func() {
			removeServices(t, client, registryServiceName)
		})

		// Start registry
		resp, err := client.RunService(ctx, api.ServiceSpec{
			Name: registryServiceName,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Image: "registry:2",
			},
			Ports: []api.PortSpec{
				{
					ContainerPort: 5000,
					PublishedPort: 5001,
					Mode:          api.PortModeHost,
					Protocol:      api.ProtocolTCP,
				},
			},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, "registry", resp.Name)

		project, err := compose.LoadProject(
			ctx,
			[]string{"fixtures/compose-build-basic/compose.yaml"},
			composecli.WithEnv([]string{
				fmt.Sprintf("TEST_REGISTRY_PORT=%s", registryHostPortStr),
			}),
		)
		require.NoError(t, err)
		servicesToBuild := cliInternal.GetServicesThatNeedBuild(project)
		serviceImage1 := fmt.Sprintf("127.0.0.1:%d/service-first", registryHostPort)
		serviceImage2 := fmt.Sprintf("127.0.0.1:%d/service-second:version2", registryHostPort)
		t.Cleanup(func() {
			// Remove the images after the test
			removeOptions := image.RemoveOptions{Force: true, PruneChildren: true}
			_, err := dockerCli.ImageRemove(ctx, serviceImage1, removeOptions)
			assert.NoErrorf(t, err, "failed to remove image %s on test cleanup", serviceImage1)
			_, err = dockerCli.ImageRemove(ctx, serviceImage2, removeOptions)
			assert.NoErrorf(t, err, "failed to remove image %s on test cleanup", serviceImage2)
		})

		servicesToBuildExpected := map[string]types.ServiceConfig{
			"service-first": {
				Name: "service-first",
				Build: &types.BuildConfig{
					Context:    path.Join(project.WorkingDir, "service-first-dir"),
					Dockerfile: "Dockerfile",
				},
				Image:       serviceImage1,
				Environment: types.NewMappingWithEquals([]string{}),
				Networks: map[string]*types.ServiceNetworkConfig{
					"default": nil,
				},
			},
			"service-second": {
				Name: "service-second",
				Build: &types.BuildConfig{
					Context:    path.Join(project.WorkingDir, "service-second-dir"),
					Dockerfile: "Dockerfile.alt",
				},
				Image:       serviceImage2,
				Environment: types.NewMappingWithEquals([]string{}),
				Networks: map[string]*types.ServiceNetworkConfig{
					"default": nil,
				},
			},
		}
		assert.Equal(t, servicesToBuildExpected, servicesToBuild)

		// Build and push the images
		buildOpts := cli.BuildOptions{
			Push:    true,
			NoCache: false,
		}
		cli.BuildServices(context.Background(), servicesToBuild, buildOpts)

		// Check the image of the first service
		ref1, err := name.NewRepository(fmt.Sprintf("127.0.0.1:%d/service-first", registryHostPort))
		require.NoError(t, err)
		tags, err := remote.List(ref1)
		require.NoError(t, err)

		assert.Equal(t, tags, []string{"latest"}, "Tags for service service-first do not match")

		// Check the image of the second service
		ref2, err := name.NewRepository(fmt.Sprintf("127.0.0.1:%d/service-second", registryHostPort))
		require.NoError(t, err)
		tags, err = remote.List(ref2)
		require.NoError(t, err)

		assert.Equal(t, tags, []string{"version2"}, "Tags for service service-second do not match")
	})
}
