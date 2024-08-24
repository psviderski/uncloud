package cmdexec

import (
	"context"
	"fmt"
	"golang.org/x/crypto/ssh"
	"strings"
)

type Remote struct {
	client *ssh.Client
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
		res := result{
			err: outErr,
		}
		if outErr == nil {
			res.out = strings.TrimSpace(string(outBytes))
		}
		done <- res
	}()

	select {
	case res := <-done:
		if res.err != nil {
			return "", fmt.Errorf("run command on remote host: %w", res.err)
		}
		return res.out, nil
	case <-ctx.Done():
		if err = session.Signal(ssh.SIGINT); err != nil {
			return "", fmt.Errorf("send interrupt signal to remote process: %w", err)
		}
		return "", fmt.Errorf("canceled: %w", ctx.Err())
	}
}

// Close closes the connection to the remote host.
func (r *Remote) Close() error {
	return r.client.Close()
}
