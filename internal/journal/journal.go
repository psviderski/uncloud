package provider

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	nodeapi "github.com/virtual-kubelet/virtual-kubelet/node/api"
)

const journalctl = "journalctl"

func journalReader(namespace, name, container string, logOpts nodeapi.ContainerLogOpts) (io.ReadCloser, func() error, error) {
	fnlog := log.
		WithField("podNamespace", namespace).
		WithField("podName", name).
		WithField("containerName", container)

	fnlog.Infof("calling for container logs with options %+v", logOpts)
	cancel := func() error { return nil } // initialize as noop

	unitName := strings.Join([]string{unitPrefix(namespace, name), container, "service"}, separator)

	// Handle all the options.
	args := []string{"-u", unitName, "--no-hostname"} // only works with -o short-xxx options.
	if logOpts.Tail > 0 {
		args = append(args, "-n")
		args = append(args, fmt.Sprintf("%d", logOpts.Tail))
	}
	if logOpts.Follow {
		args = append(args, "-f")
	}
	if !logOpts.Timestamps {
		args = append(args, "-o")
		args = append(args, "cat")
	} else {
		args = append(args, "-o")
		args = append(args, "short-full") // this is _not_ the default Go timestamp output
	}
	if logOpts.SinceSeconds > 0 {
		args = append(args, "-S")
		args = append(args, fmt.Sprintf("-%ds", logOpts.SinceSeconds))
	}
	if !logOpts.SinceTime.IsZero() {
		args = append(args, "-S")
		args = append(args, logOpts.SinceTime.Format(time.RFC3339))
	}
	// Previous might not be possible to implement
	// TODO(pires,miek) show logs from the current Pod alone https://github.com/virtual-kubelet/systemk/issues/5#issuecomment-765278538
	// LimitBytes - unsure (maybe a io.CopyBuffer?)

	fnlog.Debugf("getting container logs via: %q %v", journalctl, args)
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
				fnlog.Debugf("wait for %q failed: %s", journalctl, err)
			}
		}()
		return cmd.Process.Kill()
	}

	return p, cancel, nil
}

var ErrExpired = errors.New("timeout expired")

// journalFollow synchronously follows the io.Reader, writing each new journal entry to writer. The
// follow will continue until a single time.Time is received on the until channel (or it's closed).
func journalFollow(until <-chan time.Time, reader io.Reader, writer io.Writer) error {
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
		// When the context is Done() the 'until' channel is closed, this kicks in the defers in the GetContainerLogsHandler method.
		// this cleans up the journalctl, and closes all file descripters. Scan() then stops with an error (before any reads,
		// hence the above if err .. .isn't triggered). In the end this go-routine exits.
		// the error here is "read |0: file already closed".
	}()

	for {
		select {
		case <-until:
			return ErrExpired

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
