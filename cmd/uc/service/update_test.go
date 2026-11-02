package service

import (
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyServiceUpdate(t *testing.T) {
	t.Parallel()

	newSpec := func() api.ServiceSpec {
		return api.ServiceSpec{
			Name: "web",
			Container: api.ContainerSpec{
				Image:  "nginx",
				Labels: map[string]string{"container": "keep"},
				Resources: api.ContainerResources{
					CPU:               100 * api.MilliCore,
					Memory:            1024,
					MemoryReservation: 512,
				},
			},
			Labels: map[string]string{"tier": "frontend", "legacy": "true"},
		}
	}

	t.Run("adds, changes and removes labels", func(t *testing.T) {
		t.Parallel()

		spec := newSpec()
		updated := applyServiceUpdate(spec, updateOptions{labelRm: []string{"legacy", "missing"}},
			map[string]string{"tier": "backend", "team": "core"})

		assert.Equal(t, map[string]string{"tier": "backend", "team": "core"}, updated.Labels)
		assert.Equal(t, map[string]string{"container": "keep"}, updated.Container.Labels)
		assert.Equal(t, spec.Container.Resources, updated.Container.Resources)
		assert.Equal(t, map[string]string{"tier": "frontend", "legacy": "true"}, spec.Labels,
			"Expected the original spec to stay unchanged")
	})

	t.Run("adds labels to a spec without labels", func(t *testing.T) {
		t.Parallel()

		spec := newSpec()
		spec.Labels = nil
		updated := applyServiceUpdate(spec, updateOptions{}, map[string]string{"tier": "backend"})

		assert.Equal(t, map[string]string{"tier": "backend"}, updated.Labels)
	})

	t.Run("changes only requested limits", func(t *testing.T) {
		t.Parallel()

		opts := updateOptions{cpuChanged: true}
		require.NoError(t, opts.cpu.Set("0.5"))

		updated := applyServiceUpdate(newSpec(), opts, nil)

		assert.EqualValues(t, 500*api.MilliCore, updated.Container.Resources.CPU)
		assert.EqualValues(t, 1024, updated.Container.Resources.Memory)
		assert.EqualValues(t, 512, updated.Container.Resources.MemoryReservation)
	})

	t.Run("sends only changed fields to the container", func(t *testing.T) {
		t.Parallel()

		opts := updateOptions{memoryChanged: true}
		require.NoError(t, opts.memory.Set("2048"))

		ctr := api.ServiceContainer{ServiceSpec: newSpec()}
		containerOpts := deploy.ContainerUpdateOptions(ctr, applyServiceUpdate(ctr.ServiceSpec, opts, nil))

		require.NotNil(t, containerOpts.Memory)
		assert.EqualValues(t, 2048, *containerOpts.Memory)
		assert.Nil(t, containerOpts.CPU)
		assert.Nil(t, containerOpts.MemoryReservation)
		assert.Nil(t, containerOpts.ServiceLabels)
	})

	t.Run("removing the last label clears service labels", func(t *testing.T) {
		t.Parallel()

		ctr := api.ServiceContainer{ServiceSpec: newSpec()}
		containerOpts := deploy.ContainerUpdateOptions(ctr, applyServiceUpdate(
			ctr.ServiceSpec, updateOptions{labelRm: []string{"tier", "legacy"}}, nil,
		))

		require.NotNil(t, containerOpts.ServiceLabels)
		assert.Empty(t, *containerOpts.ServiceLabels)
	})

	t.Run("no changes produce empty container options", func(t *testing.T) {
		t.Parallel()

		ctr := api.ServiceContainer{ServiceSpec: newSpec()}
		containerOpts := deploy.ContainerUpdateOptions(ctr, applyServiceUpdate(
			ctr.ServiceSpec, updateOptions{}, map[string]string{"tier": "frontend"},
		))

		assert.Equal(t, api.UpdateContainerOptions{}, containerOpts)
	})
}
