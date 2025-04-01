package config

type Context struct {
	Name        string              `yaml:"-"`
	Connections []MachineConnection `yaml:"connections"`
}
