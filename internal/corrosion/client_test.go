package corrosion

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthRoundTripper_SetsAuthorizationHeader(t *testing.T) {
	t.Parallel()

	const token = "test-token-1234567890"

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := &AuthRoundTripper{Base: http.DefaultTransport, Token: token}
	client := &http.Client{Transport: rt}

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, "Bearer "+token, gotAuth)
	// Caller's request must not be mutated by the RoundTripper.
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestAuthRoundTripper_EmptyTokenSkipsHeader(t *testing.T) {
	t.Parallel()

	var headerSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, headerSeen = r.Header["Authorization"]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := &AuthRoundTripper{Base: http.DefaultTransport, Token: ""}
	client := &http.Client{Transport: rt}

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	assert.False(t, headerSeen, "Authorization header should not be sent when token is empty")
}
