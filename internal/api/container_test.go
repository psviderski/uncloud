package api

import (
	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestContainer_Healthy(t *testing.T) {
	t.Parallel()

	t.Run("exited", func(t *testing.T) {
		t.Parallel()
		c := &Container{Container: types.Container{
			State:  "exited",
			Status: "Exited (0) 2 minutes ago",
		}}
		assert.False(t, c.Healthy())
	})

	t.Run("running with no health check", func(t *testing.T) {
		t.Parallel()
		c := &Container{Container: types.Container{
			State:  "running",
			Status: "Up 5 minutes",
		}}
		assert.True(t, c.Healthy())
	})

	t.Run("running and healthy", func(t *testing.T) {
		t.Parallel()
		c := &Container{Container: types.Container{
			State:  "running",
			Status: "Up 3 minutes (healthy)",
		}}
		assert.True(t, c.Healthy())
	})

	t.Run("running but unhealthy", func(t *testing.T) {
		t.Parallel()
		c := &Container{Container: types.Container{
			State:  "running",
			Status: "Up 2 hours (unhealthy)",
		}}
		assert.False(t, c.Healthy())
	})

	t.Run("running with health starting", func(t *testing.T) {
		t.Parallel()
		c := &Container{Container: types.Container{
			State:  "running",
			Status: "Up 1 minute (health: starting)",
		}}
		assert.False(t, c.Healthy())
	})

	t.Run("invalid up format no time", func(t *testing.T) {
		t.Parallel()
		c := &Container{Container: types.Container{
			State:  "running",
			Status: "Up",
		}}
		assert.False(t, c.Healthy())
	})

	t.Run("invalid up format empty parentheses", func(t *testing.T) {
		t.Parallel()
		c := &Container{Container: types.Container{
			State:  "running",
			Status: "Up 5 minutes ()",
		}}
		assert.False(t, c.Healthy())
	})

	t.Run("malformed status", func(t *testing.T) {
		t.Parallel()
		c := &Container{Container: types.Container{
			State:  "running",
			Status: "Invalid status",
		}}
		assert.False(t, c.Healthy())
	})

	t.Run("restarting", func(t *testing.T) {
		t.Parallel()
		c := &Container{Container: types.Container{
			State:  "running",
			Status: "Restarting (0) 5 seconds ago",
		}}
		assert.False(t, c.Healthy())
	})
}
