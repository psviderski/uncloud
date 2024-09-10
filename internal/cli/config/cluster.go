package config

import "uncloud/internal/secret"

type Cluster struct {
	Name     string              `toml:"-"`
	Machines []MachineConnection `toml:"machines"`
	Secret   secret.Secret       `toml:"secret"`
	// UserPrivateKey is the user's WireGuard private key used to connect to cluster machines.
	UserPrivateKey secret.Secret `toml:"user_private_key"`
}
