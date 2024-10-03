package corrosion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cenkalti/backoff/v4"
	"io"
	"log/slog"
	"net/http"
	"strconv"
)

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
	resubscribe  func(ctx context.Context, fromChange uint64) (*Subscription, error)
	changes      chan *ChangeEvent
	lastChangeID uint64
	err          error
}

func newSubscription(
	ctx context.Context,
	id string,
	rows *Rows,
	body io.ReadCloser,
	decoder *json.Decoder,
	resubscribe func(ctx context.Context, fromChange uint64) (*Subscription, error),
) *Subscription {
	ctx, cancel := context.WithCancel(ctx)
	if decoder == nil {
		decoder = json.NewDecoder(body)
	}
	return &Subscription{
		ctx:         ctx,
		cancel:      cancel,
		id:          id,
		rows:        rows,
		body:        body,
		decoder:     decoder,
		resubscribe: resubscribe,
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
	go s.handleChangeEvents()

	return s.changes, nil
}

func (s *Subscription) handleChangeEvents() {
	defer s.cancel()
	defer close(s.changes)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		var e QueryEvent
		var err error
		if err = s.decoder.Decode(&e); err != nil {
			// Do not report an error that occurred due to context cancellation, just return.
			if s.ctx.Err() != nil {
				return
			}
			err = fmt.Errorf("decode query event: %w", err)
		} else if e.Error != nil {
			err = fmt.Errorf("query error: %s", *e.Error)
		} else if e.Change == nil {
			err = fmt.Errorf("expected change event, got: %+v", e)
		} else if s.lastChangeID != 0 && e.Change.ChangeID != s.lastChangeID+1 {
			// If skipRows is true, the last change ID is unknown.
			err = fmt.Errorf("missed a change: expected change ID %d, got %d",
				s.lastChangeID+1, e.Change.ChangeID)
		}

		if err == nil {
			s.lastChangeID = e.Change.ChangeID
			select {
			case s.changes <- e.Change:
			case <-s.ctx.Done():
				return
			}
		} else {
			// Report the error if resubscribing is disabled.
			if s.resubscribe == nil {
				s.err = err
				return
			}

			slog.Info("Resubscribing to Corrosion query due to an error.",
				"error", err, "id", s.id, "from_change", s.lastChangeID)
			sub, sErr := s.resubscribe(s.ctx, s.lastChangeID)
			if sErr != nil {
				// resubscribe returns a permanent error after unsuccessful retries.
				s.err = fmt.Errorf("resubscribe to query with backoff: %w", sErr)
				return
			}
			// Reset the subscription to the new one.
			s.rows = nil
			s.body = sub.body
			s.decoder = sub.decoder
			// Do not close the sub to not close the body.
			sub.cancel()
		}
	}
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

// SubscribeContext creates a subscription to receive updates for a desired SQL query. If skipRows is false,
// Subscription.Rows must be consumed before Subscription.Changes can be called. If skipRows is true, Subscription.Rows
// will return nil.
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
		return newSubscription(ctx, id, nil, resp.Body, nil, c.resubscribeWithBackoffFn(id)), nil
	}

	rows, err := newRows(ctx, resp.Body, false)
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("parse query response: %w", err)
	}
	return newSubscription(ctx, id, rows, rows.body, rows.decoder, c.resubscribeWithBackoffFn(id)), nil
}

func (c *APIClient) resubscribeWithBackoffFn(id string) func(context.Context, uint64) (*Subscription, error) {
	if c.newResubBackoff == nil {
		return nil
	}
	return func(ctx context.Context, fromChange uint64) (*Subscription, error) {
		return backoff.RetryWithData(func() (*Subscription, error) {
			slog.Debug("Retrying to resubscribe to Corrosion query.", "id", id, "from_change", fromChange)
			return c.ResubscribeContext(ctx, id, fromChange)
		}, c.newResubBackoff())
	}
}

func (c *APIClient) ResubscribeContext(ctx context.Context, id string, fromChange uint64) (*Subscription, error) {
	subURL := c.baseURL.JoinPath("/v1/subscriptions", id)
	q := subURL.Query()
	q.Set("from", strconv.FormatUint(fromChange, 10))
	subURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", subURL.String(), nil)
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

	return newSubscription(ctx, id, nil, resp.Body, nil, c.resubscribeWithBackoffFn(id)), nil
}
