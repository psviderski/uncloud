package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstallCmd(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		cmd := installCmd("root", "")
		assert.NotContains(t, cmd, "sudo")
		assert.NotContains(t, cmd, "UNCLOUD_GROUP_ADD_USER")
	})

	// Test with version
	t.Run("root with version", func(t *testing.T) {
		cmd := installCmd("root", "v1.2.3")
		assert.NotContains(t, cmd, "sudo")
		assert.NotContains(t, cmd, "UNCLOUD_GROUP_ADD_USER")
		assert.Contains(t, cmd, "UNCLOUD_VERSION=v1.2.3")
	})

	t.Run("nonroot", func(t *testing.T) {
		cmd := installCmd("nonroot", "")
		assert.Contains(t, cmd, "sudo")
		assert.Contains(t, cmd, "UNCLOUD_GROUP_ADD_USER=nonroot")
	})

	t.Run("nonroot with version", func(t *testing.T) {
		cmd := installCmd("nonroot", "v1.2.3")
		assert.Contains(t, cmd, "sudo")
		assert.Contains(t, cmd, "UNCLOUD_GROUP_ADD_USER=nonroot")
		assert.Contains(t, cmd, "UNCLOUD_VERSION=v1.2.3")
	})
}
