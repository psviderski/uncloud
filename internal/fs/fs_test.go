package fs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandHomeDir(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		assert.Equal(t, "", ExpandHomeDir(""))
	})

	t.Run("no home", func(t *testing.T) {
		assert.Equal(t, "/path", ExpandHomeDir("/path"))
	})

	t.Run("home", func(t *testing.T) {
		t.Setenv("HOME", "/home/user")
		assert.Equal(t, "/home/user/path", ExpandHomeDir("~/path"))
	})
}
