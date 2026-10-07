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

func TestParseUnixTimestamp(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Time
		ok    bool
	}{
		{
			name:  "valid with microseconds",
			input: "1769188773.687500",
			want:  time.Unix(1769188773, 687500000),
			ok:    true,
		},
		{
			name:  "zero microseconds",
			input: "1769188773.000000",
			want:  time.Unix(1769188773, 0),
			ok:    true,
		},
		{
			name:  "epoch",
			input: "0.000000",
			want:  time.Unix(0, 0),
			ok:    true,
		},
		{
			name:  "no fractional part",
			input: "1769188773",
			want:  time.Time{},
			ok:    false,
		},
		{
			name:  "non-numeric seconds",
			input: "abc.000000",
			want:  time.Time{},
			ok:    false,
		},
		{
			name:  "non-numeric microseconds",
			input: "1769188773.abc",
			want:  time.Time{},
			ok:    false,
		},
		{
			name:  "empty",
			input: "",
			want:  time.Time{},
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseUnixTimestamp([]byte(tt.input))
			assert.Equal(t, tt.ok, ok)
			assert.True(t, tt.want.Equal(got), "want %v, got %v", tt.want, got)
		})
	}
}

func TestEntry(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		timestamp time.Time
		message   string
	}{
		{
			name:      "valid entry",
			input:     "1769188773.687500 uncloudd[332455]: INFO  Starting daemon.",
			timestamp: time.Unix(1769188773, 687500000),
			message:   "uncloudd[332455]: INFO  Starting daemon.\n",
		},
		{
			name:      "unparseable timestamp keeps full line as message",
			input:     "-- Boot 1234567890 --",
			timestamp: time.Time{},
			message:   "-- Boot 1234567890 --\n",
		},
		{
			name:      "line without space keeps full line as message",
			input:     "nospacehere",
			timestamp: time.Time{},
			message:   "nospacehere\n",
		},
		{
			name:      "empty line",
			input:     "",
			timestamp: time.Time{},
			message:   "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := entry([]byte(tt.input))
			assert.True(t, tt.timestamp.Equal(got.Timestamp), "timestamp: want %v, got %v", tt.timestamp, got.Timestamp)
			assert.Equal(t, tt.message, string(got.Message))
			assert.Equal(t, api.LogStreamStdout, got.Stream)
		})
	}
}

func TestLogs(t *testing.T) {
	originalCommandContext := commandContext
	t.Cleanup(func() { commandContext = originalCommandContext })

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

func TestLogs_TimeFilters(t *testing.T) {
	originalCommandContext := commandContext
	t.Cleanup(func() { commandContext = originalCommandContext })

	var args []string
	commandContext = func(ctx context.Context, command string, commandArgs ...string) *exec.Cmd {
		assert.Equal(t, "journalctl", command)
		args = commandArgs
		return exec.CommandContext(ctx, "/usr/bin/tail", "testdata/logs")
	}

	ch, err := Logs(context.Background(), "uncloud", api.ServiceLogsOptions{
		Since: "2026-07-01T10:30:45.123456789+10:00",
		Until: "2026-07-01T01:30:45Z",
	})
	require.NoError(t, err)
	for range ch {
	}
	assert.Equal(t, []string{
		"-u", "uncloud", "--no-hostname", "-n", "0", "-o", "short-unix",
		"-S", "2026-07-01 00:30:45.123456 UTC",
		"-U", "2026-07-01 01:30:45 UTC",
	}, args)

	// Older clients and SDK callers can still pass journalctl's native filters.
	ch, err = Logs(context.Background(), "uncloud", api.ServiceLogsOptions{Since: "1h ago", Until: "today"})
	require.NoError(t, err)
	for range ch {
	}
	assert.Equal(t, []string{
		"-u", "uncloud", "--no-hostname", "-n", "0", "-o", "short-unix",
		"-S", "1h ago", "-U", "today",
	}, args)
}
