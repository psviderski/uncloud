package journal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/psviderski/uncloud/pkg/api"
)

const journalctl = "journalctl"

var commandContext = exec.CommandContext // allow override for test

func logs(ctx context.Context, unit string, opts api.ServiceLogsOptions) (io.ReadCloser, func() error, error) {
	args := []string{"-u", unit, "--no-hostname"}
	args = append(args, "-n")
	if opts.Tail > -1 {
		args = append(args, fmt.Sprintf("%d", opts.Tail))
	} else {
		args = append(args, "all")
	}
	if opts.Follow {
		args = append(args, "-f")
	}

	args = append(args, "-o")
	args = append(args, "short-unix")

	if opts.Since != "" {
		args = append(args, "-S")
		args = append(args, journalTimestamp(opts.Since))
	}
	if opts.Until != "" {
		args = append(args, "-U")
		args = append(args, journalTimestamp(opts.Until))
	}

	cmd := commandContext(ctx, journalctl, args...)
	p, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	return p, cmd.Wait, nil
}

// journalTimestamp formats normalised client timestamps for journalctl versions that do not
// accept RFC 3339 timezone suffixes. Support for timestamps containing T and Z was added in
// systemd 255 (6 December 2023). Debian 12 ships systemd 252, so it still needs this conversion.
// Journald timestamps have microsecond precision.
func journalTimestamp(value string) string {
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.UTC().Format("2006-01-02 15:04:05.999999") + " UTC"
	}
	// Preserve raw filters from older clients and SDK callers.
	return value
}

// follow synchronously follows the io.Reader, writing each new journal entry to channel.
// It stops when the reader is exhausted or the context is cancelled.
func follow(ctx context.Context, reader io.Reader, outCh chan api.LogEntry) {
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		select {
		case outCh <- entry(scanner.Bytes()):
		case <-ctx.Done():
			return
		}
	}
	if err := scanner.Err(); err != nil {
		outCh <- api.LogEntry{Err: fmt.Errorf("journal logs: %w", err)}
	}
}
