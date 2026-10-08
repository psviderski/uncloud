package caddyconfig

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testAdminServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	// macOS limits Unix socket paths to 104 bytes. t.TempDir can exceed that.
	dir, err := os.MkdirTemp("/tmp", "uc-caddy-")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	socketPath := filepath.Join(dir, "admin.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(handler)
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)

	return socketPath
}

func TestCaddyAdminClient_Validate(t *testing.T) {
	var status atomic.Int64
	status.Store(http.StatusBadRequest)
	socket := testAdminServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write([]byte(`{"message":"invalid config"}`))
	})
	client := NewCaddyAdminClient(socket)
	err := client.Validate(context.Background(), "invalid")
	var invalid *InvalidCaddyfileError
	assert.ErrorAs(t, err, &invalid)

	status.Store(http.StatusInternalServerError)
	err = client.Validate(context.Background(), "invalid")
	assert.Error(t, err)
	assert.NotErrorAs(t, err, &invalid)
}
