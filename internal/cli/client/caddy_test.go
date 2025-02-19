package client

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"uncloud/internal/api"
)

func TestClient_NewCaddyDeployment(t *testing.T) {
	t.Parallel()

	cli := &Client{}

	t.Run("latest image from Docker Hub", func(t *testing.T) {
		t.Parallel()

		deploy, err := cli.NewCaddyDeployment("", nil)
		require.NoError(t, err)

		assert.Equal(t, "caddy", deploy.Spec.Name)
		assert.Equal(t, api.ServiceModeGlobal, deploy.Spec.Mode)
		assert.Regexp(t, `^caddy:2\.\d+\.\d+$`, deploy.Spec.Container.Image)
		expectedPorts := []api.PortSpec{
			{
				PublishedPort: 80,
				ContainerPort: 80,
				Protocol:      api.ProtocolTCP,
				Mode:          api.PortModeHost,
			},
			{
				PublishedPort: 443,
				ContainerPort: 443,
				Protocol:      api.ProtocolTCP,
				Mode:          api.PortModeHost,
			},
		}
		assert.Equal(t, expectedPorts, deploy.Spec.Ports)
		// TODO:
		//assert.Equal(t, alwaysPullImage, deploy.Spec.Container.PullPolicy)
	})

	t.Run("custom image", func(t *testing.T) {
		t.Parallel()

		image := "my-caddy:1.2.3"
		deploy, err := cli.NewCaddyDeployment(image, nil)
		require.NoError(t, err)

		assert.Equal(t, "caddy", deploy.Spec.Name)
		assert.Equal(t, api.ServiceModeGlobal, deploy.Spec.Mode)
		assert.Equal(t, image, deploy.Spec.Container.Image)
		expectedPorts := []api.PortSpec{
			{
				PublishedPort: 80,
				ContainerPort: 80,
				Protocol:      api.ProtocolTCP,
				Mode:          api.PortModeHost,
			},
			{
				PublishedPort: 443,
				ContainerPort: 443,
				Protocol:      api.ProtocolTCP,
				Mode:          api.PortModeHost,
			},
		}
		assert.Equal(t, expectedPorts, deploy.Spec.Ports)
	})
}
