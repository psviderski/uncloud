package journal

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/psviderski/uncloud/pkg/api"
)

// Logs streams logs from a service and returns entries via a channel.
func Logs(ctx context.Context, unit string, opts api.ServiceLogsOptions) (<-chan api.LogEntry, error) {
	if !ValidUnit(unit) {
		return nil, fmt.Errorf("journal logs: invalid unit: %s", unit)
	}

	reader, wait, err := logs(ctx, unit, opts)
	if err != nil {
		return nil, err
	}

	outCh := make(chan api.LogEntry)

	go func() {
		defer close(outCh)
		follow(ctx, reader, outCh)
		wait()
	}()

	return outCh, nil
}

func entry(data []byte) api.LogEntry {
	// 1758193407.686964 systemd[1]: ...
	timestamp := time.Time{}
	message := data
	if timestampPart, messagePart, found := bytes.Cut(data, []byte(" ")); found {
		if t, ok := parseUnixTimestamp(timestampPart); ok {
			timestamp = t
			message = messagePart
		}
	}

	return api.LogEntry{
		Timestamp: timestamp,
		Message:   append(slices.Clone(message), '\n'), // scanner controls the buffer so Clone and re-add newline
		Stream:    api.LogStreamStdout,
	}
}

// parseUnixTimestamp parses a journalctl short-unix timestamp "SSSSSSSSSS.UUUUUU" with microsecond precision.
func parseUnixTimestamp(b []byte) (time.Time, bool) {
	secPart, usecPart, found := bytes.Cut(b, []byte("."))
	if !found {
		return time.Time{}, false
	}
	sec, err := strconv.ParseInt(string(secPart), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	usec, err := strconv.ParseInt(string(usecPart), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(sec, usec*1000), true
}
