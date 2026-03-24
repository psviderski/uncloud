package journal

import (
	"context"
	"os/exec"
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
)

func TestLogs(t *testing.T) {
	commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/usr/bin/tail", "testdata/logs")
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch, err := Logs(ctx, "notused", api.ServiceLogsOptions{})
	if err != nil {
		t.Fatalf("got error from Logs: %s", err)
	}

	// testdata/logs is 6 lines.
	i := 0
	for range ch {
		i++
		// TODO(miek): check entry's content?
	}
	if i != 6 {
		t.Fatalf("expected %d entries, got %d", 6, i)
	}
}
