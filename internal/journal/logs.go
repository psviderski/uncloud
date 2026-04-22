package journal

import (
	"bytes"
	"context"
	"fmt"
	"slices"
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
	// 2025-10-12T11:03:27+02:00 systemd[1]:
	timestamp := time.Time{}
	message := data
	if len(data) > 30 && data[4] == '-' && data[7] == '-' && data[10] == 'T' {
		timestampPart, messagePart, found := bytes.Cut(data, []byte(" "))
		var err error
		if found {
			timestamp, err = time.Parse(time.RFC3339Nano, string(timestampPart))
			if err != nil {
				timestamp = time.Time{}
			}
			message = messagePart
		}
	}

	return api.LogEntry{
		Timestamp: timestamp,
		Message:   append(slices.Clone(message), '\n'), // scanner controls the buffer so Clone and re-add newline
		Stream:    api.LogStreamStdout,
	}
}
