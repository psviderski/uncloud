package sshexec

import (
	"fmt"
	"net"
	"os"
	osuser "os/user"
	"strconv"
	"time"

	"github.com/psviderski/uncloud/internal/fs"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func Connect(user, host string, port int, sshKeyPath string) (*ssh.Client, error) {
	// Use the current OS user if no user is specified to be make it consistent with ssh CLI behavior.
	if user == "" {
		if u, err := osuser.Current(); err == nil {
			user = u.Username
		}
	}
	if port == 0 {
		port = 22
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	// Try to connect using SSH agent only.
	agentAuth, agentClose, agentErr := sshAgentAuth()
	if agentErr == nil {
		defer agentClose()
		config := &ssh.ClientConfig{
			User:            user,
			Auth:            []ssh.AuthMethod{agentAuth},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         5 * time.Second,
		}
		var client *ssh.Client
		if client, agentErr = ssh.Dial("tcp", addr, config); agentErr == nil {
			return client, nil
		}
	}
	// Fall back to using private key as the connection attempt using SSH agent failed.
	if sshKeyPath == "" {
		// TODO: iterate over ~/.ssh/id_* and try to connect using each key.
		return nil, fmt.Errorf("connect using SSH agent: %w", agentErr)
	}

	keyAuth, err := privateKeyAuth(sshKeyPath)
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{keyAuth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("connect using private key %q: %w", sshKeyPath, err)
	}

	return client, nil
}

func sshAgentAuth() (ssh.AuthMethod, func(), error) {
	conn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return nil, func() {}, fmt.Errorf("connect to SSH agent: %w", err)
	}
	auth := ssh.PublicKeysCallback(agent.NewClient(conn).Signers)
	connClose := func() { _ = conn.Close() }
	return auth, connClose, nil
}

func privateKeyAuth(path string) (ssh.AuthMethod, error) {
	path = fs.ExpandHomeDir(path)
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key file %q: %w", path, err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	// TODO: prompt password for the private key if needed.
	//  Check: https://github.com/alexellis/k3sup/blob/master/cmd/install.go
	return ssh.PublicKeys(signer), nil
}
