package containerd

import (
	"fmt"
	"time"

	"github.com/containerd/containerd/v2/client"
)

// Client is a containerd client for image operations.
type Client struct {
	Client *client.Client
}

// NewClient creates a new containerd client connected to the given socket path.
func NewClient(sockPath string) (*Client, error) {
	if sockPath == "" {
		return nil, fmt.Errorf("containerd socket path is required")
	}

	c, err := client.New(
		sockPath,
		client.WithDefaultNamespace("moby"),
		client.WithTimeout(60*time.Second),
	)
	if err != nil {
		return nil, err
	}

	return &Client{Client: c}, nil
}

// Close closes the containerd client connection.
func (c *Client) Close() error {
	if c.Client != nil {
		return c.Client.Close()
	}
	return nil
}
