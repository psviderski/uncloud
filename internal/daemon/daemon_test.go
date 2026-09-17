package daemon

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectActivatedListener(t *testing.T) {
	t.Run("no activated listeners", func(t *testing.T) {
		listener, err := selectActivatedListener(nil)
		require.NoError(t, err)
		assert.Nil(t, listener)
	})

	t.Run("only listener regardless of name", func(t *testing.T) {
		activated := testListener(t)

		listener, err := selectActivatedListener(map[string][]net.Listener{"another.socket": {activated}})
		require.NoError(t, err)
		assert.Same(t, activated, listener)
	})

	t.Run("named listener when multiple", func(t *testing.T) {
		activated := testListener(t)
		extra := testListener(t)

		listener, err := selectActivatedListener(map[string][]net.Listener{
			systemdSocketUnit: {activated},
			"another.socket":  {extra},
		})
		require.NoError(t, err)
		assert.Same(t, activated, listener)
		assert.Error(t, extra.Close())
	})

	t.Run("multiple listeners without expected name", func(t *testing.T) {
		first := testListener(t)
		second := testListener(t)

		listener, err := selectActivatedListener(map[string][]net.Listener{
			"first.socket":  {first},
			"second.socket": {second},
		})
		require.ErrorContains(t, err, "expected one systemd-activated socket")
		assert.Nil(t, listener)
	})

	t.Run("multiple listeners under expected name", func(t *testing.T) {
		first := testListener(t)
		second := testListener(t)

		listener, err := selectActivatedListener(map[string][]net.Listener{
			systemdSocketUnit: {first, second},
		})
		require.ErrorContains(t, err, "expected one systemd-activated socket")
		assert.Nil(t, listener)
	})
}

func testListener(t *testing.T) net.Listener {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	return listener
}
