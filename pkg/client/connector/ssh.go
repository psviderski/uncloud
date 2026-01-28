package connector

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/psviderski/uncloud/internal/machine"
	"github.com/psviderski/uncloud/internal/sshexec"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type SSHConnectorConfig struct {
	User    string
	Host    string
	Port    int
	KeyPath string

	SockPath string
}

// Destination returns the destination string that can be passed to ssh CLI in the format [user@]host.
func (cfg *SSHConnectorConfig) Destination() string {
	dst := cfg.Host
	if cfg.User != "" {
		dst = fmt.Sprintf("%s@%s", cfg.User, dst)
	}

	return dst
}

// SSHConnector establishes a connection to the machine API through an SSH tunnel to the machine.
type SSHConnector struct {
	config SSHConnectorConfig
	client *ssh.Client
}

func NewSSHConnector(cfg *SSHConnectorConfig) *SSHConnector {
	return &SSHConnector{config: *cfg}
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
			dst := c.config.Destination()
			if c.config.Port != 0 {
				dst = fmt.Sprintf("%s:%d", dst, c.config.Port)
			}
			return nil, fmt.Errorf("SSH login to %s: %w", dst, err)
		}
	}

	sockPath := c.config.SockPath
	if sockPath == "" {
		sockPath = machine.DefaultUncloudSockPath
	}
	conn, err := grpc.NewClient(
		"unix://"+sockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(defaultServiceConfig),
		grpc.WithContextDialer(
			func(ctx context.Context, addr string) (net.Conn, error) {
				addr = strings.TrimPrefix(addr, "unix://")
				conn, dErr := c.client.DialContext(ctx, "unix", addr)
				if dErr != nil {
					return nil, fmt.Errorf(
						"connect to machine API socket '%s' through SSH tunnel (is uncloud.service running "+
							"on the remote machine and does the SSH user '%s' have permissions to access the socket?):"+
							" %w",
						addr, c.client.User(), dErr,
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

// Dialer returns a proxy dialer for establishing connections within the cluster through the SSH tunnel.
// The connector must be created with an existing SSH client or Connect must be called first.
func (c *SSHConnector) Dialer() (proxy.ContextDialer, error) {
	if c.client == nil {
		return nil, errors.New("SSH connection must be established first")
	}
	return c.client, nil
}

func (c *SSHConnector) Close() error {
	if c.client != nil {
		err := c.client.Close()
		c.client = nil
		return err
	}
	return nil
}
