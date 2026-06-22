package corrosion

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient builds an APIClient pointed at srv with a fast resubscribe backoff for tests.
func newTestClient(t *testing.T, srv *httptest.Server) *APIClient {
	t.Helper()

	baseURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	return &APIClient{
		baseURL: baseURL,
		client:  srv.Client(),
		newResubBackoff: func() backoff.BackOff {
			return backoff.NewExponentialBackOff(
				backoff.WithInitialInterval(time.Millisecond),
				backoff.WithMaxInterval(10*time.Millisecond),
				backoff.WithMaxElapsedTime(5*time.Second),
			)
		},
	}
}

func flushString(t *testing.T, w http.ResponseWriter, s string) {
	t.Helper()
	_, err := w.Write([]byte(s))
	require.NoError(t, err)
	w.(http.Flusher).Flush()
}

// TestSubscription_ResubscribeFromZeroDrainsSnapshot verifies that when the connection drops before any
// change is seen (lastChangeID == 0) and Corrosion replays the full query snapshot on resubscription from
// change 0, the change handler drains the snapshot and delivers the subsequent change instead of looping on
// "expected change event, got: {Columns:...}".
// The change of behaviour in Corrosion: https://github.com/superfly/corrosion/pull/355
func TestSubscription_ResubscribeFromZeroDrainsSnapshot(t *testing.T) {
	t.Parallel()

	const snapshot = `{"columns":["id","info"]}
{"row":[1,["a","b"]]}
{"eoq":{"time":1e-7,"change_id":0}}
`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			// Initial subscription: send the snapshot, then end the stream to force a resubscribe.
			w.Header().Set("corro-query-id", "test-sub")
			w.WriteHeader(http.StatusOK)
			flushString(t, w, snapshot)
		case http.MethodGet:
			// Resubscription from change 0: Corrosion v1.0.0+ replays the full snapshot, then changes.
			require.Equal(t, "0", r.URL.Query().Get("from"))
			w.Header().Set("corro-query-id", "test-sub")
			w.WriteHeader(http.StatusOK)
			flushString(t, w, snapshot+`{"change":["insert",2,["c","d"],1]}`+"\n")
			// Keep the connection open so the handler doesn't end the stream and trigger another resubscribe.
			<-r.Context().Done()
		default:
			require.FailNow(t, fmt.Sprintf("unexpected request method: %s", r.Method))
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := client.SubscribeContext(ctx, "SELECT id, info FROM machines", nil, false)
	require.NoError(t, err)

	// Consume the initial rows so Changes can be called.
	rows := sub.Rows()
	for rows.Next() {
	}
	require.NoError(t, rows.Err())

	changes, err := sub.Changes()
	require.NoError(t, err)

	select {
	case change := <-changes:
		require.NotNil(t, change, "changes channel closed with error: %v", sub.Err())
		assert.Equal(t, ChangeTypeInsert, change.Type)
		assert.Equal(t, uint64(2), change.RowID)
		assert.Equal(t, uint64(1), change.ChangeID)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for a change after resubscribe", sub.Err())
	}
}

// TestSubscription_ResubscribeFromNonZeroSkipsSnapshot verifies that a resubscription from a non-zero change
// streams changes directly, without a replayed snapshot, and the change handler delivers them.
func TestSubscription_ResubscribeFromNonZeroSkipsSnapshot(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			// Initial subscription with a non-zero last change id, then end the stream to force a resubscribe.
			w.Header().Set("corro-query-id", "test-sub")
			w.WriteHeader(http.StatusOK)
			flushString(t, w, `{"columns":["id","info"]}`+"\n"+
				`{"row":[1,["a","b"]]}`+"\n"+
				`{"eoq":{"time":1e-7,"change_id":5}}`+"\n")
		case http.MethodGet:
			// Resubscription from change 5: only changes are streamed, no snapshot.
			require.Equal(t, "5", r.URL.Query().Get("from"))
			require.False(t, strings.Contains(r.URL.RawQuery, "skip_rows"))
			w.WriteHeader(http.StatusOK)
			flushString(t, w, `{"change":["update",1,["a","b2"],6]}`+"\n")
			<-r.Context().Done()
		default:
			require.FailNow(t, fmt.Sprintf("unexpected request method: %s", r.Method))
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := client.SubscribeContext(ctx, "SELECT id, info FROM machines", nil, false)
	require.NoError(t, err)

	rows := sub.Rows()
	for rows.Next() {
	}
	require.NoError(t, rows.Err())

	changes, err := sub.Changes()
	require.NoError(t, err)

	select {
	case change := <-changes:
		require.NotNil(t, change, "changes channel closed with error: %v", sub.Err())
		assert.Equal(t, ChangeTypeUpdate, change.Type)
		assert.Equal(t, uint64(6), change.ChangeID)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for a change after resubscribe", sub.Err())
	}
}
