package caddyconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
)

// CaddyAdminClient is a client for interacting with the Caddy admin API over a Unix socket.
type CaddyAdminClient struct {
	socketPath string
	client     *http.Client
}

func NewCaddyAdminClient(socketPath string) *CaddyAdminClient {
	return &CaddyAdminClient{
		socketPath: socketPath,
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
		},
	}
}

// Adapt converts a Caddyfile to JSON configuration without loading or running it.
func (c *CaddyAdminClient) Adapt(ctx context.Context, caddyfile string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/adapt", strings.NewReader(caddyfile))
	if err != nil {
		return "", fmt.Errorf("create adapt request: %w", err)
	}
	req.Header.Set("Content-Type", "text/caddyfile")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send adapt request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode == http.StatusOK {
		// Parse the response body to extract the result field.
		var msg struct {
			Result json.RawMessage `json:"result"`
		}
		if err = json.Unmarshal(body, &msg); err != nil {
			return "", fmt.Errorf("parse adapt response: %w", err)
		}
		return string(msg.Result), nil
	}

	// If the response is a 400 Bad Request, try to parse the error message from it.
	if resp.StatusCode == http.StatusBadRequest {
		var apiError caddy.APIError
		if err = json.Unmarshal(body, &apiError); err == nil {
			return "", errors.New(apiError.Message)
		}
	}

	return "", errors.New(string(body))
}

// Load loads a Caddyfile configuration into the Caddy instance running on the machine.
// Due to a Caddy bug (https://github.com/caddyserver/caddy/issues/7246), we first adapt the Caddyfile to JSON
// and then load the JSON config to get proper error handling.
func (c *CaddyAdminClient) Load(ctx context.Context, caddyfile string) error {
	jsonConfig, err := c.Adapt(ctx, caddyfile)
	if err != nil {
		return fmt.Errorf("adapt Caddyfile to JSON config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/load", strings.NewReader(jsonConfig))
	if err != nil {
		return fmt.Errorf("create load request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send load request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	// If the response is a 400 Bad Request, try to parse the error message from it.
	if resp.StatusCode == http.StatusBadRequest {
		var apiError caddy.APIError
		if err = json.Unmarshal(body, &apiError); err == nil {
			return fmt.Errorf("caddy responded with error: %s", apiError.Message)
		}
	}

	return fmt.Errorf("caddy responded with error: HTTP %d: %s", resp.StatusCode, string(body))
}

// Validate checks if the provided Caddyfile can be adapted to Caddy JSON config using the running Caddy instance via
// its admin API. It doesn't guarantee that the Caddyfile is actually valid and can be loaded. For example, a tls
// directive with a missing certificate will pass the adaptation but will fail when Caddy tries to load it.
// But this is the best we can do over the admin API.
// TODO: run 'docker exec caddy-container caddy validate' to do proper validation or implement a Caddy module that
// exposes a validation endpoint.
func (c *CaddyAdminClient) Validate(ctx context.Context, caddyfile string) error {
	_, err := c.Adapt(ctx, caddyfile)
	return err
}
