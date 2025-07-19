package sshexec

import (
	"context"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/ssh"
)

type Remote struct {
	client *ssh.Client
}

func NewRemote(client *ssh.Client) *Remote {
	return &Remote{client: client}
}

// Run runs the command on the remote host and returns its output with all leading and trailing
// white space removed.
func (r *Remote) Run(ctx context.Context, cmd string) (string, error) {
	session, err := r.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	defer func() {
		_ = session.Close()
	}()

	// Run the command in a goroutine to be able to cancel it.
	type result struct {
		out string
		err error
	}
	done := make(chan result)
	go func() {
		outBytes, outErr := session.CombinedOutput(cmd)
		done <- result{
			out: strings.TrimSpace(string(outBytes)),
			err: outErr,
		}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			return res.out, fmt.Errorf("run command on remote host: %w: %s", res.err, res.out)
		}
		return res.out, nil
	case <-ctx.Done():
		if err = session.Signal(ssh.SIGINT); err != nil {
			return "", fmt.Errorf("send interrupt signal to remote process: %w", err)
		}
		return "", fmt.Errorf("canceled: %w", ctx.Err())
	}
}

// Stream runs the command on the remote host and streams its output to the provided writers.
func (r *Remote) Stream(ctx context.Context, cmd string, stdout, stderr io.Writer) error {
	session, err := r.client.NewSession()
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	defer func() {
		_ = session.Close()
	}()

	session.Stdout, session.Stderr = stdout, stderr
	// Run the command in a goroutine to be able to cancel it.
	done := make(chan error)
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case err = <-done:
		if err != nil {
			return fmt.Errorf("run command on remote host: %w", err)
		}
		return nil
	case <-ctx.Done():
		if err = session.Signal(ssh.SIGINT); err != nil {
			return fmt.Errorf("send interrupt signal to remote process: %w", err)
		}
		return fmt.Errorf("canceled: %w", ctx.Err())
	}
}

// Close closes the connection to the remote host.
func (r *Remote) Close() error {
	return r.client.Close()
}
