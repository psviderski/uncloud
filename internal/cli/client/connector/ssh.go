package connector

import (
	"context"
	"fmt"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"strings"
	"uncloud/internal/machine"
	"uncloud/internal/sshexec"
)

type SSHConnectorConfig struct {
	User    string
	Host    string
	Port    int
	KeyPath string

	APISockPath string
}

// SSHConnector establishes a connection to the machine API through an SSH tunnel to the machine.
type SSHConnector struct {
	config SSHConnectorConfig
	client *ssh.Client
}

func NewSSHConnector(cfg *SSHConnectorConfig) *SSHConnector {
	c := &SSHConnector{config: *cfg}
	if c.config.User == "" {
		c.config.User = "root"
	}
	if c.config.Port == 0 {
		c.config.Port = 22
	}
	if c.config.APISockPath == "" {
		c.config.APISockPath = machine.DefaultAPISockPath
	}
	return c
}

func NewSSHConnectorFromClient(client *ssh.Client) *SSHConnector {
	return &SSHConnector{client: client}
}

// TODO: handle context cancelation.
func (c *SSHConnector) Connect(ctx context.Context) (*grpc.ClientConn, error) {
	if c.client == nil {
		// Establish an SSH connection if the SSH client is not provided.
		if c.config == (SSHConnectorConfig{}) {
			return nil, fmt.Errorf("SSH connector not configured")
		}
		var err error
		c.client, err = sshexec.Connect(c.config.User, c.config.Host, c.config.Port, c.config.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("SSH login to %s@%s:%d: %w", c.config.User, c.config.Host, c.config.Port, err)
		}
	}

	conn, err := grpc.NewClient(
		"unix://"+c.config.APISockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(
			func(ctx context.Context, addr string) (net.Conn, error) {
				addr = strings.TrimPrefix(addr, "unix://")
				conn, dErr := c.client.DialContext(ctx, "unix", addr)
				if dErr != nil {
					return nil, fmt.Errorf(
						"connect to machine API socket %s through SSH tunnel (is the Uncloud daemon running "+
							"on the remote machine and does the SSH user have permissions to access the socket?): %w",
						addr, dErr,
					)
				}
				return conn, nil
			},
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create machine API client: %w", err)
	}
	return conn, nil
}

func (c *SSHConnector) Close() error {
	if c.client != nil {
		err := c.client.Close()
		c.client = nil
		return err
	}
	return nil
}
