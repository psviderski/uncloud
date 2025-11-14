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

const (
	DefaultSSHUser = "root"
	DefaultSSHPort = 22
)

type MachineConnection struct {
	SSH        SSHDestination `yaml:"ssh,omitempty"`
	SSHCLI     SSHDestination `yaml:"ssh_cli,omitempty"`
	SSHKeyFile string         `yaml:"ssh_key_file,omitempty"`
	// TCP is the address and port of the machine's API server.
	// The pointer is used to omit the field when not set. Otherwise, yaml marshalling includes an empty object.
	TCP       *netip.AddrPort `yaml:"tcp,omitempty"`
	Host      string          `yaml:"host,omitempty"`
	PublicKey secret.Secret   `yaml:"public_key,omitempty"`
	Name      string          `yaml:"name,omitempty"`
}

func (c MachineConnection) String() string {
	var connStr string
	if c.SSH != "" {
		connStr = "ssh://" + string(c.SSH)
	} else if c.SSHCLI != "" {
		connStr = "ssh+cli://" + string(c.SSHCLI)
	} else if c.TCP != nil && c.TCP.IsValid() {
		connStr = fmt.Sprintf("tcp://%s", c.TCP)
	}

	if c.Name != "" {
		if connStr != "" {
			return fmt.Sprintf("%s (%s)", c.Name, connStr)
		}
		return c.Name
	}

	if connStr != "" {
		return connStr
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

	if setCount == 0 {
		return errors.New("no connection method specified (ssh, ssh_cli, or tcp required)")
	}
	if setCount > 1 {
		return errors.New("only one connection method allowed per connection (ssh, ssh_cli, or tcp)")
	}

	return nil
}

// SSHDestination represents an SSH destination string in the canonical form of "user@host:port".
// The default user "root" and port 22 can be omitted.
type SSHDestination string

func NewSSHDestination(user, host string, port int) SSHDestination {
	dst := host
	if port != 0 && port != DefaultSSHPort {
		dst = net.JoinHostPort(host, strconv.Itoa(port))
	}
	if user == "" {
		user = DefaultSSHUser
	}
	dst = user + "@" + dst
	return SSHDestination(dst)
}

func (d SSHDestination) Parse() (user string, host string, port int, err error) {
	host = string(d)
	if strings.Contains(host, "@") {
		user, host, _ = strings.Cut(host, "@")
	}
	if user == "" {
		user = DefaultSSHUser
	}
	h, p, sErr := net.SplitHostPort(host)
	if sErr == nil {
		host = h
		port, err = strconv.Atoi(p)
	}
	if port == 0 {
		port = DefaultSSHPort
	}
	return
}
