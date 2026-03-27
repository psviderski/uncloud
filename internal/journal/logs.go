package journal

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/psviderski/uncloud/pkg/api"
)

// Logs streams logs from a service and returns entries via a channel.
func Logs(ctx context.Context, unit string, opts api.ServiceLogsOptions) (<-chan api.LogEntry, error) {
	reader, err := logs(ctx, unit, opts)
	if err != nil {
		return nil, err
	}

	outCh := make(chan api.LogEntry)

	switch opts.Follow {
	case false:

		go func() {
			defer close(outCh)

			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				outCh <- entry(scanner.Bytes())
			}

			if err := scanner.Err(); err != nil {
				outCh <- api.LogEntry{Err: fmt.Errorf("journal logs: %w", err)}
			}
		}()

	case true:
		go func() {
			defer close(outCh)

			err := follow(ctx, reader, outCh)
			if err != nil {
				outCh <- api.LogEntry{Err: fmt.Errorf("journal logs: %w", err)}
			}
		}()
	}

	go func() {
		<-ctx.Done()
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
		Message:   slices.Clone(message), // scanner controls the buffer
		Stream:    api.LogStreamStdout,
	}
}
