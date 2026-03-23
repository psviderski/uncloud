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

func logs(unit string, opts api.ServiceLogsOptions) (io.ReadCloser, func() error, error) {
	cancel := func() error { return nil }

	args := []string{"-u", unit, "--no-hostname"}
	if opts.Tail > 0 {
		args = append(args, "-n")
		args = append(args, fmt.Sprintf("%d", opts.Tail))
	}
	if opts.Follow {
		args = append(args, "-f")
	}

	args = append(args, "-o")
	args = append(args, "short-iso")

	if opts.Since != "" {
		args = append(args, "-S")
		args = append(args, opts.Since)
	}

	cmd := exec.Command(journalctl, args...)
	p, err := cmd.StdoutPipe()
	if err != nil {
		return nil, cancel, err
	}

	if err := cmd.Start(); err != nil {
		return nil, cancel, err
	}

	cancel = func() error {
		go func() {
			if err := cmd.Wait(); err != nil {
				// log, error?
			}
		}()
		return cmd.Process.Kill()
	}

	return p, cancel, nil
}

// follow synchronously follows the io.Reader, writing each new journal entry to writer. The
// follow will continue until a single time.Time is received on the until channel (or it's closed).
func follow(ctx context.Context, reader io.Reader, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)
	bufch := make(chan []byte)
	errch := make(chan error)

	go func() {
		for scanner.Scan() {
			if err := scanner.Err(); err != nil {
				errch <- err
				return
			}
			bufch <- scanner.Bytes()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil

		case err := <-errch:
			return err

		case buf := <-bufch:
			if _, err := writer.Write(buf); err != nil {
				return err
			}
			if _, err := io.WriteString(writer, "\n"); err != nil {
				return err
			}
		}
	}
}
