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

// CaddyAdminValidator validates Caddyfile via the Caddy admin API.
type CaddyAdminValidator struct {
	socketPath string
	client     *http.Client
}

func NewCaddyAdminValidator(socketPath string) *CaddyAdminValidator {
	return &CaddyAdminValidator{
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

// Validate checks if the provided Caddyfile can be adapted to Caddy JSON config using the running Caddy instance via
// its admin API. It doesn't guarantee that the Caddyfile is actually valid and can be loaded. For example, a tls
// directive with a missing certificate will pass the adaptation but will fail when Caddy tries to load it.
// But this is the best we can do over the admin API.
// TODO: run 'docker exec caddy-container caddy validate' to do proper validation or implement a Caddy module that
// exposes a validation endpoint.
func (c *CaddyAdminValidator) Validate(ctx context.Context, caddyfile string) error {
	// Bogus host is used so that http.NewRequest is happy but it doesn't matter since we're using a Unix socket.
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/adapt", strings.NewReader(caddyfile))
	if err != nil {
		return fmt.Errorf("create adapt request: %w", err)
	}
	req.Header.Set("Content-Type", "text/caddyfile")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send adapt request: %w", err)
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
			return errors.New(apiError.Message)
		}
	}

	return errors.New(string(body))
}
