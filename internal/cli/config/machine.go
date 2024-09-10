package config

import (
	"net"
	"strconv"
	"strings"
	"uncloud/internal/secret"
)

const (
	DefaultSSHUser = "root"
	DefaultSSHPort = 22
)

type MachineConnection struct {
	SSH       SSHDestination `toml:"ssh,omitempty"`
	Host      string         `toml:"host,omitempty"`
	PublicKey secret.Secret  `toml:"public_key,omitempty"`
}

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
	if strings.Contains(string(d), "@") {
		user, host, _ = strings.Cut(string(d), "@")
	}
	h, p, sErr := net.SplitHostPort(host)
	if sErr == nil {
		host = h
		port, err = strconv.Atoi(p)
	}
	return
}
