package config

import "uncloud/internal/secret"

type Cluster struct {
	Name        string              `toml:"-"`
	Connections []MachineConnection `toml:"connections"`
	// UserPrivateKey is the user's WireGuard private key used to connect to cluster machines.
	UserPrivateKey secret.Secret `toml:"user_private_key"`
}
