package corrosion

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/net/http2"
	"io"
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

type Statement struct {
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

// Exec writes changes to the Corrosion database for propagation through the cluster. Corrosion does not sync schema
// changes made using this method. Use Corrosion's schema_files to create and update the cluster's database schema.
func (c *APIClient) Exec(ctx context.Context, query string, args ...any) (*ExecResult, error) {
	statements := []Statement{
		{
			Query:  query,
			Params: args,
		},
	}
	resp, err := c.ExecMulti(ctx, statements...)
	if err != nil {
		return nil, err
	}

	if len(resp.Results) == 0 {
		return nil, fmt.Errorf("no results: %+v", resp)
	}
	return &resp.Results[0], nil
}

// ExecMulti writes changes to the Corrosion database for propagation through the cluster.
// Unlike Exec, this method allows multiple statements to be executed in a single transaction.
func (c *APIClient) ExecMulti(ctx context.Context, statements ...Statement) (*ExecResponse, error) {
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
