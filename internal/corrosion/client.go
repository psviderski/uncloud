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
	"time"
)

const (
	// HTTP2ConnectTimeout is the maximum amount of time a client will wait for a connection to be established.
	http2ConnectTimeout = 3 * time.Second
	// HTTP2Timeout is the maximum amount of time a client will wait for a response.
	http2Timeout = 20 * time.Second
)

// APIClient is a client for the Corrosion API.
type APIClient struct {
	baseURL *url.URL
	client  *http.Client
}

func NewAPIClient(addr netip.AddrPort) (*APIClient, error) {
	baseURL, err := url.Parse(fmt.Sprintf("http://%s", addr))
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	return &APIClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: http2Timeout,
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
				Backoff: backoff.NewExponentialBackOff(
					backoff.WithInitialInterval(100*time.Millisecond),
					backoff.WithMaxInterval(1*time.Second),
					backoff.WithMaxElapsedTime(10*time.Second),
				),
			},
		},
	}, nil
}

type RetryRoundTripper struct {
	Base    http.RoundTripper
	Backoff backoff.BackOff
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
	boff := backoff.WithContext(rt.Backoff, req.Context())
	return backoff.RetryWithData(roundTrip, boff)
}

type Statement struct {
	Query  string `json:"query"`
	Params []any  `json:"params"`
}

type ExecResponse struct {
	Results []ExecResult `json:"results"`
	Time    float64      `json:"time"`
	Version *uint        `json:"version"`
}

type ExecResult struct {
	RowsAffected uint    `json:"rows_affected"`
	Time         float64 `json:"time"`
	Error        *string `json:"error"`
}

// ExecContext writes changes to the Corrosion database for propagation through the cluster. The args are for any
// placeholder parameters in the query. Corrosion does not sync schema changes made using this method. Use Corrosion's
// schema_files to create and update the cluster's database schema.
func (c *APIClient) ExecContext(ctx context.Context, query string, args ...any) (*ExecResult, error) {
	statements := []Statement{
		{
			Query:  query,
			Params: args,
		},
	}
	resp, err := c.ExecMultiContext(ctx, statements...)
	if err != nil {
		return nil, err
	}

	if len(resp.Results) == 0 {
		return nil, fmt.Errorf("no results: %+v", resp)
	}
	return &resp.Results[0], nil
}

// ExecMultiContext writes changes to the Corrosion database for propagation through the cluster.
// Unlike ExecContext, this method allows multiple statements to be executed in a single transaction.
func (c *APIClient) ExecMultiContext(ctx context.Context, statements ...Statement) (*ExecResponse, error) {
	body, err := json.Marshal(statements)
	if err != nil {
		return nil, fmt.Errorf("marshal queries: %w", err)
	}

	transactionsURL := c.baseURL.JoinPath("/v1/transactions").String()
	req, err := http.NewRequestWithContext(ctx, "POST", transactionsURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	var execResp ExecResponse
	if resp.StatusCode == http.StatusOK {
		if err = json.NewDecoder(resp.Body).Decode(&execResp); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
		// The response may still contain DB errors even if the status code is OK. Return them along with the response.
		var errs []error
		for _, result := range execResp.Results {
			if result.Error != nil {
				errs = append(errs, errors.New(*result.Error))
			}
		}
		return &execResp, errors.Join(errs...)
	} else if resp.StatusCode == http.StatusInternalServerError {
		// If the response is an Internal Server Error, the response body may contain the error encoded as ExecResponse.
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}
		if err = json.Unmarshal(respBody, &execResp); err != nil {
			return nil, fmt.Errorf("internal server error: %s", respBody)
		}
		if len(execResp.Results) > 0 && execResp.Results[0].Error != nil {
			return nil, errors.New(*execResp.Results[0].Error)
		}
		return nil, fmt.Errorf("internal server error: %s", respBody)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, respBody)
}

type QueryEvent struct {
	Columns []string    `json:"columns"`
	Row     *RowEvent   `json:"row"`
	EOQ     *EndOfQuery `json:"eoq"`
	// TODO: implement event type Change to support subscriptions.
	//Change []any       `json:"change"`
	// Error is a server-side error that occurred during query execution. It's considered fatal for the client
	// as it cannot be recovered from server-side.
	Error *string `json:"error"`
}

type EndOfQuery struct {
	Time     float64 `json:"time"`
	ChangeID *uint64 `json:"change_id"`
}

type RowEvent struct {
	RowID  uint64
	Values []json.RawMessage
}

func (re *RowEvent) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("invalid row event: %w", err)
	}
	if len(raw) != 2 {
		return fmt.Errorf("invalid row event: expected an array of 2 elements")
	}
	if err := json.Unmarshal(raw[0], &re.RowID); err != nil {
		return fmt.Errorf("invalid row event: %w", err)
	}
	if err := json.Unmarshal(raw[1], &re.Values); err != nil {
		return fmt.Errorf("invalid row event: %w", err)
	}
	return nil
}

func (re *RowEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{re.RowID, re.Values})
}

// QueryContext executes a query that returns rows, typically a SELECT.
// The args are for any placeholder parameters in the query.
func (c *APIClient) QueryContext(ctx context.Context, query string, args ...any) (*Rows, error) {
	statement := Statement{
		Query:  query,
		Params: args,
	}
	body, err := json.Marshal(statement)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	queriesURL := c.baseURL.JoinPath("/v1/queries").String()
	req, err := http.NewRequestWithContext(ctx, "POST", queriesURL, bytes.NewReader(body))
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

	rows, err := newRows(ctx, resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("parse query response: %w", err)
	}
	return rows, nil
}

// Rows is the result of a query. Its cursor starts before the first row of the result set.
// Use [Rows.Next] to advance from row to row.
type Rows struct {
	ctx     context.Context
	body    io.ReadCloser
	decoder *json.Decoder

	columns []string
	row     RowEvent
	time    float64
	err     error
}

func newRows(ctx context.Context, body io.ReadCloser) (*Rows, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	decoder := json.NewDecoder(body)
	var e QueryEvent
	if err := decoder.Decode(&e); err != nil {
		return nil, fmt.Errorf("decode query event: %w", err)
	}
	if e.Columns == nil {
		return nil, fmt.Errorf("expected columns event, got: %+v", e)
	}

	return &Rows{
		ctx:     ctx,
		body:    body,
		decoder: decoder,
		columns: e.Columns,
	}, nil
}

// Columns returns the column names.
func (rs *Rows) Columns() []string {
	return rs.columns
}

// Next prepares the next result row for reading with the [Rows.Scan] method. It returns true on success, or false
// if there is no next result row or an error happened while preparing it. [Rows.Err] should be consulted to distinguish
// between the two cases.
//
// Every call to [Rows.Scan], even the first one, must be preceded by a call to [Rows.Next].
func (rs *Rows) Next() bool {
	select {
	case <-rs.ctx.Done():
		rs.err = rs.ctx.Err()
		_ = rs.Close()
		return false
	default:
	}

	var e QueryEvent
	if err := rs.decoder.Decode(&e); err != nil {
		rs.err = fmt.Errorf("decode query event: %w", err)
		_ = rs.Close()
		return false
	}
	// Server-side query error.
	if e.Error != nil {
		rs.err = fmt.Errorf("query error: %s", *e.Error)
		_ = rs.Close()
		return false
	}

	if e.Row != nil {
		if len(e.Row.Values) != len(rs.columns) {
			rs.err = fmt.Errorf("expected %d column values, got %d", len(rs.columns), len(e.Row.Values))
			_ = rs.Close()
			return false
		}
		rs.row = *e.Row
		return true
	}
	if e.EOQ != nil {
		rs.time = e.EOQ.Time
		_ = rs.Close()
		return false
	}

	rs.err = fmt.Errorf("expected row or eof event, got: %+v", e)
	_ = rs.Close()
	return false
}

// Err returns the error, if any, that was encountered during iteration.
// Err may be called after an explicit or implicit [Rows.Close].
func (rs *Rows) Err() error {
	return rs.err
}

// Scan copies the columns in the current row into the values pointed at by dest.
// The number of values in dest must be the same as the number of columns in [Rows].
// Scan converts JSON-encoded column values to the provided Go types using [json.Unmarshal].
func (rs *Rows) Scan(dest ...any) error {
	if rs.err != nil {
		return rs.err
	}
	if len(dest) != len(rs.columns) {
		return fmt.Errorf("expected %d values, got %d", len(rs.columns), len(dest))
	}

	for i, v := range rs.row.Values {
		if err := json.Unmarshal(v, dest[i]); err != nil {
			return fmt.Errorf("unmarshal column value #%d: %w", i, err)
		}
	}
	return nil
}

// Time returns the time taken to execute the query in seconds. It's only available after all rows have been consumed.
// It doesn't include the time to send the query, receive the response, or iterate over the rows.
func (rs *Rows) Time() (float64, error) {
	if rs.time == 0 {
		if rs.Err() != nil {
			return 0, fmt.Errorf("time is not available: %w", rs.Err())
		}
		return 0, errors.New("time is not available until all rows are consumed")
	}
	return rs.time, nil
}

// Close closes the [Rows], preventing further enumeration. If [Rows.Next] is called and returns false,
// the [Rows] are closed automatically and it will suffice to check the result of [Rows.Err].
// Close is idempotent and does not affect the result of [Rows.Err].
func (rs *Rows) Close() error {
	return rs.body.Close()
}
