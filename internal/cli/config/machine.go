package config

type MachineConnection struct {
	User   string `toml:"user,omitempty"`
	Host   string `toml:"host"`
	Port   int    `toml:"port"`
	SSHKey string `toml:"ssh_key,omitempty"`
}
