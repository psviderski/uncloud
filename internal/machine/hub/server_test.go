package hub

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAPIServer_NoRegisteredServices(t *testing.T) {
	t.Parallel()

	server := NewAPIServer(nil)
	t.Cleanup(server.Stop)

	// The allowlist is a stream interceptor that unary methods of registered services would skip, so all requests
	// must go through the unknown service handler.
	assert.Empty(t, server.GetServiceInfo())
}
