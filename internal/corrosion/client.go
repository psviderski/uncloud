package corrosion

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cenkalti/backoff/v4"
	"golang.org/x/net/http2"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
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

// SubscribeContext creates a subscription to receive updates for a desired SQL query. If skipRows is false,
// Subscription.Rows must be consumed before Subscription.Changes can be called. If skipRows is true, Subscription.Rows
// will be nil.
func (c *APIClient) SubscribeContext(
	ctx context.Context, query string, args []any, skipRows bool,
) (*Subscription, error) {
	statement := Statement{
		Query:  query,
		Params: args,
	}
	body, err := json.Marshal(statement)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	subURL := c.baseURL.JoinPath("/v1/subscriptions")
	if skipRows {
		q := subURL.Query()
		q.Set("skip_rows", "true")
		subURL.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "POST", subURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, respBody)
	}

	id := resp.Header.Get("corro-query-id")
	if id == "" {
		resp.Body.Close()
		return nil, errors.New("missing corro-query-id header in response")
	}

	if skipRows {
		return newSubscription(ctx, id, nil, resp.Body, nil), nil
	}

	rows, err := newRows(ctx, resp.Body, false)
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("parse query response: %w", err)
	}
	return newSubscription(ctx, id, rows, rows.body, rows.decoder), nil
}

func (c *APIClient) ResubscribeContext(
	ctx context.Context, id string, skipRows bool, fromChange uint64,
) (*Subscription, error) {
	// TODO
	return nil, nil
}

type ChangeType string

var (
	ChangeTypeInsert ChangeType = "insert"
	ChangeTypeUpdate ChangeType = "update"
	ChangeTypeDelete ChangeType = "delete"
)

type ChangeEvent struct {
	Type     ChangeType
	RowID    uint64
	Values   []json.RawMessage
	ChangeID uint64
}

func (ce *ChangeEvent) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("invalid change event: %w", err)
	}
	if len(raw) != 4 {
		return fmt.Errorf("invalid change event: expected an array of 4 elements")
	}
	if err := json.Unmarshal(raw[0], &ce.Type); err != nil {
		return fmt.Errorf("invalid change event type: %w", err)
	}
	if err := json.Unmarshal(raw[1], &ce.RowID); err != nil {
		return fmt.Errorf("invalid change event row ID: %w", err)
	}
	if err := json.Unmarshal(raw[2], &ce.Values); err != nil {
		return fmt.Errorf("invalid change event values: %w", err)
	}
	if err := json.Unmarshal(raw[3], &ce.ChangeID); err != nil {
		return fmt.Errorf("invalid change event change ID: %w", err)
	}
	return nil
}

func (ce *ChangeEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{ce.Type, ce.RowID, ce.Values, ce.ChangeID})
}

// Scan copies the column values in the change event into the values pointed at by dest.
// The number of values in dest must be the same as the number of columns in the change.
// Scan converts JSON-encoded column values to the provided Go types using [json.Unmarshal].
func (ce *ChangeEvent) Scan(dest ...any) error {
	if len(dest) != len(ce.Values) {
		return fmt.Errorf("expected %d values, got %d", len(ce.Values), len(dest))
	}

	for i, v := range ce.Values {
		if err := json.Unmarshal(v, dest[i]); err != nil {
			return fmt.Errorf("unmarshal column value #%d: %w", i, err)
		}
	}
	return nil
}

// Subscription receives updates from the Corrosion database for a desired SQL query.
type Subscription struct {
	ctx    context.Context
	cancel context.CancelFunc

	id           string
	rows         *Rows
	body         io.ReadCloser
	decoder      *json.Decoder
	changes      chan *ChangeEvent
	lastChangeID uint64
	err          error
}

func newSubscription(
	ctx context.Context, id string, rows *Rows, body io.ReadCloser, decoder *json.Decoder,
) *Subscription {
	ctx, cancel := context.WithCancel(ctx)
	if decoder == nil {
		decoder = json.NewDecoder(body)
	}
	return &Subscription{
		id:      id,
		rows:    rows,
		ctx:     ctx,
		cancel:  cancel,
		body:    body,
		decoder: decoder,
	}
}

// ID returns the subscription ID.
func (s *Subscription) ID() string {
	return s.id
}

// Rows returns the rows of the query or nil if skipRows was true when creating the subscription or if the subscription
// was created with [APIClient.ResubscribeContext].
func (s *Subscription) Rows() *Rows {
	return s.rows
}

// Changes returns a channel that receives change events for the query. Changes are not available until all rows
// are consumed. The channel is closed when the context is done, or an error occurs while reading the changes,
// or when the subscription is closed explicitly. If it's closed due to an error, [Subscription.Err] will return
// the error.
func (s *Subscription) Changes() (<-chan *ChangeEvent, error) {
	if s.changes != nil {
		return s.changes, nil
	}

	if s.rows != nil {
		if s.rows.eoq == nil {
			return nil, errors.New("changes are not available until all rows are consumed")
		}
		s.lastChangeID = *s.rows.eoq.ChangeID
	}
	s.changes = make(chan *ChangeEvent)

	go func() {
		// Close the body when the context is done to unblock the decoder in the following goroutine.
		<-s.ctx.Done()
		s.body.Close()
	}()

	go func() {
		defer s.cancel()
		defer close(s.changes)

		for {
			select {
			case <-s.ctx.Done():
				return
			default:
			}

			var e QueryEvent
			if err := s.decoder.Decode(&e); err != nil {
				// Do not report an error that occurred due to context cancellation.
				if s.ctx.Err() == nil {
					s.err = fmt.Errorf("decode query event: %w", err)
				}
				return
			}

			if e.Error != nil {
				s.err = fmt.Errorf("query error: %s", *e.Error)
				return
			}
			if e.Change == nil {
				s.err = fmt.Errorf("expected change event, got: %+v", e)
				return
			}
			// If skipRows is true, the last change ID is unknown.
			if s.lastChangeID != 0 && e.Change.ChangeID != s.lastChangeID+1 {
				s.err = fmt.Errorf("missed a change: expected change ID %d, got %d",
					s.lastChangeID+1, e.Change.ChangeID)
				return
			}

			s.lastChangeID = e.Change.ChangeID
			select {
			case s.changes <- e.Change:
			case <-s.ctx.Done():
				return
			}
		}
	}()

	return s.changes, nil
}

// Err returns the error, if any, that was encountered during fetching changes.
// Err may be called after an explicit or implicit [Subscription.Close].
func (s *Subscription) Err() error {
	return s.err
}

func (s *Subscription) Close() error {
	s.cancel()
	return s.body.Close()
}
