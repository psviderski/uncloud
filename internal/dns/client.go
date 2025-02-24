package dns

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// The dns package code is based on https://github.com/acorn-io/runtime/blob/main/pkg/dns.

// Client handles interactions with the Uncloud DNS API service.
type Client interface {
	// ReserveDomain calls Uncloud DNS to reserve a new domain. It returns the domain, a token for authentication,
	// and an error.
	ReserveDomain(endpoint string) (string, string, error)
}

// ErrAuthNoDomain indicates that a request failed authentication because the domain was not found.
// If encountered, a new domain needs to be reserved.
var ErrAuthNoDomain = errors.New("the supplied domain failed authentication")

// NewClient creates a new AcornDNS client
func NewClient() Client {
	return &client{
		c: http.DefaultClient,
	}
}

type client struct {
	c *http.Client
}

func (c *client) ReserveDomain(endpoint string) (string, string, error) {
	url := fmt.Sprintf("%s/%s", endpoint, "domains")

	req, err := c.request(http.MethodPost, url, nil, "")
	if err != nil {
		return "", "", err
	}

	resp := &DomainResponse{}
	err = c.do(req, resp)
	if err != nil {
		return "", "", err
	}

	domain := resp.Name
	if strings.HasPrefix(domain, ".") {
		domain = domain[1:]
	}
	return domain, resp.Token, nil
}

func (c *client) request(method string, url string, body io.Reader, token string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")

	if token != "" {
		bearer := "Bearer " + token
		req.Header.Add("Authorization", bearer)
	}

	return req, nil
}

func (c *client) do(req *http.Request, responseBody any) error {
	slog.Debug("Making request to DNS service.", "method", req.Method, "url", req.URL)

	resp, err := c.c.Do(req)
	if err != nil {
		return err
	}

	slog.Debug("Response code for request to DNS service.",
		"method", req.Method, "url", req.URL, "code", resp.StatusCode)
	// When err is nil, resp contains a non-nil resp.Body which must be closed.
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		var authError AuthErrorResponse

		err = json.Unmarshal(body, &authError)
		if err != nil {
			return fmt.Errorf("unmarshal auth error response: %w", err)
		}

		if authError.Data.NoDomain {
			return ErrAuthNoDomain
		}

		return errors.New("authentication failed")
	}

	if code := resp.StatusCode; code < 200 || code > 300 {
		return fmt.Errorf("unexpected response status code: %d", code)
	}

	if responseBody != nil {
		err = json.Unmarshal(body, responseBody)
		if err != nil {
			return fmt.Errorf("unmarshal response body (%s): %w", string(body), err)
		}
	}

	return nil
}
