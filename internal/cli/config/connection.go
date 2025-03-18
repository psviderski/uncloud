package config

import (
	"net"
	"net/netip"
	"strconv"
	"strings"
	"uncloud/internal/secret"
)

const (
	DefaultSSHUser = "root"
	DefaultSSHPort = 22
)

type MachineConnection struct {
	SSH        SSHDestination `toml:"ssh,omitempty"`
	SSHKeyFile string         `toml:"ssh_key_file,omitempty"`
	TCP        netip.AddrPort `toml:"tcp,omitempty"`
	Host       string         `toml:"host,omitempty"`
	PublicKey  secret.Secret  `toml:"public_key,omitempty"`
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
