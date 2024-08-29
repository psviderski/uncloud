package config

import "uncloud/internal/secret"

type MachineConnection struct {
	User      string        `toml:"user,omitempty"`
	Host      string        `toml:"host"`
	Port      int           `toml:"port"`
	SSHKey    string        `toml:"ssh_key,omitempty"`
	PublicKey secret.Secret `toml:"public_key,omitempty"`
}
