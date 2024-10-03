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
	baseURL              *url.URL
	client               *http.Client
	newResubsribeBackoff func() backoff.BackOff
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
		newResubsribeBackoff: func() backoff.BackOff {
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

// WithResubscribeBackoff sets the backoff policy for resubscribing to a query.
func WithResubscribeBackoff(newBackoff func() backoff.BackOff) APIClientOption {
	return func(c *APIClient) {
		c.newResubsribeBackoff = newBackoff
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
				slog.Debug("Retrying corrosion API request due to network error", "error", err)
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
