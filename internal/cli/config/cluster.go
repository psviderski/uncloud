package config

import "uncloud/internal/secret"

type Cluster struct {
	Name     string              `toml:"-"`
	Machines []MachineConnection `toml:"machines"`
	Secret   secret.Secret       `toml:"secret"`
}
