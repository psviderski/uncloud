package journal

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogs(t *testing.T) {
	commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/usr/bin/tail", "testdata/logs")
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch, err := Logs(ctx, "uncloud", api.ServiceLogsOptions{})
	require.NoError(t, err)

	i := 0
	for range ch {
		i++
	}
	assert.Equal(t, 6, i)

	commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/usr/bin/tail", "-f", "testdata/logs")
	}

	ctx = context.Background()
	ctx, cancel = context.WithCancel(ctx)
	go func() { time.Sleep(1 * time.Second); cancel() }()

	ch, err = Logs(ctx, "uncloud", api.ServiceLogsOptions{Tail: 3})
	require.NoError(t, err)

	i = 0
	for range ch {
		i++
	}
	// Still six because heartbeats are not written here and Tail is ignored as the command is overridden.
	assert.Equal(t, 6, i)
}
