package corrosion

import (
	"context"
	"crypto/tls"
	"fmt"
	"golang.org/x/net/http2"
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
	http2Timeout = 30 * time.Second
)

type APIClient struct {
	baseURL *url.URL
	client  *http.Client
}

type ExecResponse struct {
	Results []ExecResult `json:"results"`
	Time    float64      `json:"time"`
	Version *uint64      `json:"version"`
}

type ExecResult struct {
	RowsAffected *uint    `json:"rows_affected"`
	Time         *float64 `json:"time"`
	Error        *string  `json:"error"`
}

type statement struct {
	Query  string `json:"query"`
	Params []any  `json:"params"`
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
			Transport: &http2.Transport{
				AllowHTTP: true,
				DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
					dialer := &net.Dialer{
						Timeout: http2ConnectTimeout,
					}
					return dialer.DialContext(ctx, network, addr)
				},
			},
		},
	}, nil
}
