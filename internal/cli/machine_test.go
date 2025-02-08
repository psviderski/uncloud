package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstallCmd(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		cmd := installCmd("root")
		assert.NotContains(t, cmd, "sudo")
	})

	t.Run("nonroot", func(t *testing.T) {
		cmd := installCmd("nonroot")
		assert.Contains(t, cmd, "sudo")
	})
}
