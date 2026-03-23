package journal

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/psviderski/uncloud/pkg/api"
)

// Logs streams logs from a service and returns entries via a channel.
func Logs(ctx context.Context, unit string, opts api.ServiceLogsOptions) (<-chan api.LogEntry, error) {
	reader, cancel, err := logs(unit, opts)
	if err != nil {
		return nil, err
	}

	outCh := make(chan api.LogEntry)

	switch opts.Follow {
	case false:

		go func() {
			defer close(outCh)
			defer cancel()

			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				outCh <- entry(scanner.Bytes())
			}

			if err := scanner.Err(); err != nil {
				outCh <- api.LogEntry{Err: fmt.Errorf("journal logs: %w", err)}
			}
		}()

	case true:
		fw := &FollowWriter{outCh}

		go func() {
			defer close(outCh)

			err := follow(ctx, reader, fw)
			if err != nil {
				outCh <- api.LogEntry{Err: fmt.Errorf("journal logs: %w", err)}
			}
		}()
	}

	go func() {
		<-ctx.Done()
		reader.Close()
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
			timestamp, err = time.Parse(time.RFC3339, string(timestampPart))
			if err != nil {
				timestamp = time.Time{}
			}
			message = messagePart
		}
	}

	return api.LogEntry{
		Timestamp: timestamp,
		Message:   message,
		Stream:    api.LogStreamStdout,
	}
}

type FollowWriter struct {
	ch chan api.LogEntry
}

func (fw *FollowWriter) Write(p []byte) (int, error) {
	fw.ch <- entry(p)
	return len(p), nil
}
