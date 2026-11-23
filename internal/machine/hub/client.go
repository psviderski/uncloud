package hub

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/psviderski/uncloud/api/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Client is a client for the Hub API.
type Client struct {
	pb.HubClient
	conn *grpc.ClientConn
}

// NewClient creates a client for the Hub API endpoint. It connects on the first call.
func NewClient(endpoint string) (*Client, error) {
	target, useTLS, err := parseEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(transportCredentials(useTLS)))
	if err != nil {
		return nil, fmt.Errorf("create gRPC client: %w", err)
	}

	return &Client{
		HubClient: pb.NewHubClient(conn),
		conn:      conn,
	}, nil
}

// Close closes the connection to Hub.
func (c *Client) Close() error {
	return c.conn.Close()
}

// parseEndpoint returns the gRPC target to dial for the Hub endpoint and whether to use TLS. The endpoint must be
// an http or https URL without a path.
func parseEndpoint(endpoint string) (target string, useTLS bool, err error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", false, fmt.Errorf("parse Hub endpoint: %w", err)
	}

	port := u.Port()
	switch strings.ToLower(u.Scheme) {
	case "https":
		useTLS = true
		if port == "" {
			port = "443"
		}
	case "http":
		if port == "" {
			port = "80"
		}
	default:
		return "", false, fmt.Errorf("invalid Hub endpoint '%s': scheme must be http or https", endpoint)
	}

	if u.Hostname() == "" {
		return "", false, fmt.Errorf("invalid Hub endpoint '%s': host is empty", endpoint)
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", false, fmt.Errorf("invalid Hub endpoint '%s': only scheme, host and port are allowed", endpoint)
	}

	return net.JoinHostPort(u.Hostname(), port), useTLS, nil
}

func transportCredentials(useTLS bool) credentials.TransportCredentials {
	if useTLS {
		// TLS only verifies Hub using the system root certificates.
		return credentials.NewClientTLSFromCert(nil, "")
	}
	return insecure.NewCredentials()
}
