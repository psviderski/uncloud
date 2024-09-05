package config

import (
	"uncloud/internal/secret"
)

type MachineConnection struct {
	Host      string        `toml:"host"`
	PublicKey secret.Secret `toml:"public_key"`
}
