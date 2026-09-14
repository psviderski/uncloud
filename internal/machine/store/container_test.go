package store

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNormaliseContainerForStore_StripsEnv pins the behaviour that all fields carrying environment variables
// are cleared before a container record is written to the replicated store, regardless of which part of the
// spec they live in. See https://github.com/psviderski/uncloud/issues/422.
func TestNormaliseContainerForStore_StripsEnv(t *testing.T) {
	t.Parallel()

	newContainer := func(preDeploy *api.PreDeployHook) api.ServiceContainer {
		return api.ServiceContainer{
			Container: api.Container{
				InspectResponse: container.InspectResponse{
					Config: &container.Config{
						Env: []string{"CONTROL_TOKEN=control-value-aaa"},
					},
				},
			},
			ServiceSpec: api.ServiceSpec{
				Container: api.ContainerSpec{
					Env: api.EnvVars{"CONTROL_TOKEN": "control-value-aaa"},
				},
				PreDeploy: preDeploy,
			},
		}
	}

	t.Run("service with pre-deploy hook", func(t *testing.T) {
		t.Parallel()

		ctr := newContainer(&api.PreDeployHook{
			Command: []string{"/bin/true"},
			Env:     api.EnvVars{"HOOK_TOKEN": "hook-value-bbb"},
		})

		normaliseContainerForStore(&ctr)

		assert.Nil(t, ctr.Config.Env, "Config.Env should be stripped")
		assert.Nil(t, ctr.ServiceSpec.Container.Env, "ServiceSpec.Container.Env should be stripped")
		require.NotNil(t, ctr.ServiceSpec.PreDeploy, "PreDeploy itself should be left in place")
		assert.Nil(t, ctr.ServiceSpec.PreDeploy.Env, "ServiceSpec.PreDeploy.Env should be stripped")
		// Non-env fields of the hook must survive the normalisation.
		assert.Equal(t, []string{"/bin/true"}, ctr.ServiceSpec.PreDeploy.Command)
	})

	t.Run("service without pre-deploy hook", func(t *testing.T) {
		t.Parallel()

		ctr := newContainer(nil)

		require.NotPanics(t, func() {
			normaliseContainerForStore(&ctr)
		})

		assert.Nil(t, ctr.Config.Env)
		assert.Nil(t, ctr.ServiceSpec.Container.Env)
		assert.Nil(t, ctr.ServiceSpec.PreDeploy)
	})
}
