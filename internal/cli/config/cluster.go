package config

type Cluster struct {
	Name        string              `toml:"-"`
	Connections []MachineConnection `toml:"connections"`
}
