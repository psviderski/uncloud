package corrosion

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/cenkalti/backoff/v4"
	"golang.org/x/net/http2"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

const (
	// http2ConnectTimeout is the maximum amount of time an HTTP2 client will wait for a connection to be established.
	http2ConnectTimeout = 3 * time.Second
	// http2MaxRetryTime is the maximum amount of time an HTTP2 client will retry a request.
	http2MaxRetryTime = 10 * time.Second
	// resubscribeMaxRetryTime is the maximum amount of time an API client will retry resubscribing to a query after
	// an error occurs.
	resubscribeMaxRetryTime = 60 * time.Second
)

// APIClient is a client for the Corrosion API.
type APIClient struct {
	baseURL         *url.URL
	client          *http.Client
	newResubBackoff func() backoff.BackOff
}

// NewAPIClient creates a new Corrosion API client. The client retries on network errors using an exponential backoff
// policy with a maximum interval of 1 second and a maximum elapsed time of 10 seconds.
// It automatically resubscribes to active subscriptions if an error occurs using an exponential backoff policy with a
// maximum interval of 1 second and a maximum elapsed time of 60 seconds.
// Use the WithHTTP2Client option to provide a custom HTTP client and the WithResubscribeBackoff option to change the
// backoff policy for resubscribing to a query.
func NewAPIClient(addr netip.AddrPort, opts ...APIClientOption) (*APIClient, error) {
	baseURL, err := url.Parse(fmt.Sprintf("http://%s", addr))
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	c := &APIClient{
		baseURL: baseURL,
		client: &http.Client{
			Transport: &RetryRoundTripper{
				Base: &http2.Transport{
					AllowHTTP: true,
					DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
						dialer := &net.Dialer{
							Timeout: http2ConnectTimeout,
						}
						return dialer.DialContext(ctx, network, addr)
					},
				},
				NewBackoff: func() backoff.BackOff {
					return backoff.NewExponentialBackOff(
						backoff.WithInitialInterval(100*time.Millisecond),
						backoff.WithMaxInterval(1*time.Second),
						backoff.WithMaxElapsedTime(http2MaxRetryTime),
					)
				},
			},
		},
		newResubBackoff: func() backoff.BackOff {
			return backoff.NewExponentialBackOff(
				backoff.WithInitialInterval(100*time.Millisecond),
				backoff.WithMaxInterval(1*time.Second),
				backoff.WithMaxElapsedTime(resubscribeMaxRetryTime),
			)
		},
	}

	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

type APIClientOption func(*APIClient)

func WithHTTP2Client(client *http.Client) APIClientOption {
	return func(c *APIClient) {
		c.client = client
	}
}

// WithResubscribeBackoff sets the backoff policy for resubscribing to a query if an error occurs.
// Pass nil to disable resubscribing.
func WithResubscribeBackoff(newBackoff func() backoff.BackOff) APIClientOption {
	return func(c *APIClient) {
		c.newResubBackoff = newBackoff
	}
}

type RetryRoundTripper struct {
	Base http.RoundTripper
	// NewBackoff creates a new backoff policy for each request.
	NewBackoff func() backoff.BackOff
}

func (rt *RetryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	roundTrip := func() (*http.Response, error) {
		resp, err := rt.Base.RoundTrip(req)
		if err != nil {
			var opErr *net.OpError
			if errors.As(err, &opErr) {
				// Not certain, but I expect operational errors should generally be retryable.
				slog.Debug("Retrying corrosion API request due to network error.", "error", err)
				return nil, err
			}
			// Don't retry on other errors.
			return nil, backoff.Permanent(err)
		}
		// Success, don't retry.
		return resp, err
	}
	boff := backoff.WithContext(rt.NewBackoff(), req.Context())
	return backoff.RetryWithData(roundTrip, boff)
}

type RetrySubscription struct {
	ctx    context.Context
	cancel context.CancelFunc

	client       *APIClient
	sub          *Subscription
	changes      chan *ChangeEvent
	lastChangeID uint64
	err          error
	backoff      backoff.BackOff
}

func NewRetrySubscription(ctx context.Context, client *APIClient, sub *Subscription, boff backoff.BackOff) *RetrySubscription {
	ctx, cancel := context.WithCancel(ctx)
	if boff == nil {
		boff = backoff.NewExponentialBackOff(
			backoff.WithInitialInterval(100*time.Millisecond),
			backoff.WithMaxInterval(1*time.Second),
			backoff.WithMaxElapsedTime(60*time.Second),
		)
	}
	return &RetrySubscription{
		ctx:     ctx,
		cancel:  cancel,
		client:  client,
		sub:     sub,
		backoff: backoff.WithContext(boff, ctx),
	}
}

func (rs *RetrySubscription) ID() string {
	return rs.sub.ID()
}

func (rs *RetrySubscription) Changes() (<-chan *ChangeEvent, error) {
	if rs.changes != nil {
		return rs.changes, nil
	}

	// Ensure the rows are consumed if they have been requested.
	changes, err := rs.sub.Changes()
	if err != nil {
		return nil, err
	}
	rs.lastChangeID = rs.sub.lastChangeID

	go func() {
		defer close(rs.changes)

		var change *ChangeEvent
		for {
			select {
			case change = <-changes:
			case <-rs.ctx.Done():
				return
			}

			if change != nil {
				select {
				case rs.changes <- change:
				case <-rs.ctx.Done():
					return
				}
				rs.lastChangeID = change.ChangeID
				continue
			}

			// The underlying subscription has been closed due to an error or context cancellation.
			// Return if the context is done or try to resubscribe otherwise.
			if rs.ctx.Err() != nil {
				return
			}

			if err = rs.resubscribe(); err != nil {
				// resubscribe returns a permanent error after unsuccessful retries.
				rs.err = fmt.Errorf("resubscribe to query: %w", err)
				return
			}
			changes, err = rs.sub.Changes()
			if err != nil {
				// Unexpected error but report it anyway.
				rs.err = err
				return
			}
		}
	}()

	return rs.changes, nil
}

func (rs *RetrySubscription) resubscribe() error {
	return backoff.Retry(func() error {
		slog.Debug("Resubscribing to Corrosion query.", "id", rs.ID(), "from_change", rs.lastChangeID)
		sub, err := rs.client.ResubscribeContext(rs.ctx, rs.ID(), rs.lastChangeID)
		if err != nil {
			return err
		}
		rs.sub = sub
		return nil
	}, rs.backoff)
}

func (rs *RetrySubscription) Err() error {
	return rs.err
}

func (rs *RetrySubscription) Close() {
	rs.cancel()
}
