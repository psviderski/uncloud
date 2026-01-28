package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/psviderski/uncloud/internal/secret"
)

type MachineConnection struct {
	SSH        SSHDestination `yaml:"ssh,omitempty"`
	SSHCLI     SSHDestination `yaml:"ssh_cli,omitempty"`
	SSHKeyFile string         `yaml:"ssh_key_file,omitempty"`
	// TCP is the address and port of the machine's API server.
	// The pointer is used to omit the field when not set. Otherwise, yaml marshalling includes an empty object.
	TCP *netip.AddrPort `yaml:"tcp,omitempty"`
	// Unix is the path to the machine's API unix socket.
	Unix      string        `yaml:"unix,omitempty"`
	Host      string        `yaml:"host,omitempty"`
	PublicKey secret.Secret `yaml:"public_key,omitempty"`
	MachineID string        `yaml:"machine_id,omitempty"`
}

func (c *MachineConnection) String() string {
	if c.SSH != "" {
		return "ssh://" + string(c.SSH)
	} else if c.SSHCLI != "" {
		return "ssh+cli://" + string(c.SSHCLI)
	} else if c.TCP != nil && c.TCP.IsValid() {
		return fmt.Sprintf("tcp://%s", c.TCP)
	} else if c.Unix != "" {
		return fmt.Sprintf("unix://%s", c.Unix)
	}
	return "unknown connection"
}

func (c *MachineConnection) Validate() error {
	setCount := 0
	if c.SSH != "" {
		setCount++
	}
	if c.SSHCLI != "" {
		setCount++
	}
	if c.TCP != nil && c.TCP.IsValid() {
		setCount++
	}
	if c.Unix != "" {
		setCount++
	}

	if setCount == 0 {
		return errors.New("no connection method specified (ssh, ssh_cli, tcp, or unix required)")
	}
	if setCount > 1 {
		return errors.New("only one connection method allowed per connection (ssh, ssh_cli, tcp, or unix)")
	}

	return nil
}

// SSHDestination represents an SSH destination string in the canonical form of "user@host:port".
// Empty user or port components are omitted.
type SSHDestination string

// NewSSHDestination constructs an SSHDestination from user, host, and port components.
// If user is empty, it is omitted.
// If port is 0, it is omitted.
func NewSSHDestination(user, host string, port int) SSHDestination {
	dst := host
	if port != 0 {
		dst = net.JoinHostPort(host, strconv.Itoa(port))
	}
	if user != "" {
		dst = fmt.Sprintf("%s@%s", user, dst)
	}

	return SSHDestination(dst)
}

// Parse parses the SSH destination string into user, host, and port components.
// If user is not specified, it returns an empty string.
// If port is not specified, it returns 0.
func (d SSHDestination) Parse() (user string, host string, port int, err error) {
	host = string(d)
	if strings.Contains(host, "@") {
		user, host, _ = strings.Cut(host, "@")
	}
	h, p, sErr := net.SplitHostPort(host)
	if sErr == nil {
		host = h
		port, err = strconv.Atoi(p)
	}

	return
}
