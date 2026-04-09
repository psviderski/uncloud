package journal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/psviderski/uncloud/pkg/api"
)

const (
	UnitUncloud   = "uncloud"
	UnitDocker    = "docker"
	UnitCorrosion = "uncloud-corrosion"
)

func ValidUnit(unit string) bool {
	switch unit {
	case UnitUncloud:
	case UnitDocker:
	case UnitCorrosion:
	default:
		return false
	}
	return true
}

const journalctl = "journalctl"

var commandContext = exec.CommandContext // allow override for test

func logs(ctx context.Context, unit string, opts api.ServiceLogsOptions) (io.ReadCloser, error) {
	if !ValidUnit(unit) {
		return nil, fmt.Errorf("journal logs: invalid unit: %s", unit)
	}
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
	args = append(args, "short-iso-precise")

	if opts.Since != "" {
		args = append(args, "-S")
		args = append(args, opts.Since)
	}
	if opts.Until != "" {
		args = append(args, "-U")
		args = append(args, opts.Until)
	}

	cmd := commandContext(ctx, journalctl, args...)
	p, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return p, nil
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
