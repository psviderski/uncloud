package journal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/psviderski/uncloud/pkg/api"
)

const journalctl = "journalctl"

var commandContext = exec.CommandContext // overidable for the test

func logs(ctx context.Context, unit string, opts api.ServiceLogsOptions) (io.ReadCloser, error) {
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

// follow synchronously follows the io.Reader, writing each new journal entry to writer. The
// follow will continue until a single time.Time is received on the until channel (or it's closed).
func follow(ctx context.Context, reader io.Reader, outCh chan api.LogEntry) error {
	scanner := bufio.NewScanner(reader)

	go func() {
		for scanner.Scan() {
			outCh <- entry(scanner.Bytes())
		}
		if err := scanner.Err(); err != nil {
			outCh <- api.LogEntry{Err: fmt.Errorf("journal logs: %w", err)}
		}
		return
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		}
	}
}
