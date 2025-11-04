package docker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/docker/docker/client"
)

type Client struct {
	*client.Client
}

// WaitDaemonReady waits for the Docker daemon to start and be ready to serve requests.
// TODO(lhf): convert into a Client method.
func WaitDaemonReady(ctx context.Context, cli *client.Client) error {
	// Retry to ping the Docker daemon until it's ready or the context is canceled.
	boff := backoff.WithContext(backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(100*time.Millisecond),
		backoff.WithMaxInterval(1*time.Second),
		backoff.WithMaxElapsedTime(0),
	), ctx)

	waitingLogged := false
	ping := func() error {
		_, err := cli.Ping(ctx)
		if err == nil {
			if waitingLogged {
				slog.Info("Docker daemon is ready.")
			}
			return nil
		}
		if !client.IsErrConnectionFailed(err) {
			return backoff.Permanent(fmt.Errorf("connect to Docker daemon: %w", err))
		}

		if !waitingLogged {
			slog.Info("Waiting for Docker daemon to start and be ready.")
			waitingLogged = true
		}
		return err
	}

	if err := backoff.Retry(ping, boff); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return fmt.Errorf("ping Docker: %w", err)
	}
	return nil
}
