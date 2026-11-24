package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/psviderski/uncloud/internal/corrosion/corrotest"
	"github.com/stretchr/testify/require"
)

// corro is the Corrosion instance running in Docker that all tests in this package share.
var corro = &corrotest.Shared{Schema: Schema}

func TestMain(m *testing.M) {
	code := m.Run()
	if err := corro.Terminate(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to terminate shared Corrosion: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// newTestKeyspace returns a keyspace in the shared Corrosion with a unique namespace, so the test can isolate records
// from other tests.
func newTestKeyspace(t *testing.T) *Keyspace {
	t.Helper()

	ks, err := New(corro.Client(t), nil).Keyspace("test-" + strings.ToLower(rand.Text()))
	require.NoError(t, err)

	return ks
}
