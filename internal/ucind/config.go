package ucind

import (
	"fmt"

	"github.com/psviderski/uncloud/internal/cli/config"
)

type ConfigUpdater struct {
	path string
}

func NewConfigUpdater(path string) *ConfigUpdater {
	return &ConfigUpdater{path: path}
}

func (u *ConfigUpdater) AddCluster(c Cluster) error {
	cfg, err := config.NewFromFile(u.path)
	if err != nil {
		return fmt.Errorf("read Uncloud config: %w", err)
	}

	if _, ok := cfg.Contexts[c.Name]; ok {
		return fmt.Errorf("cluster context '%s' already exists", c.Name)
	}

	clusterCfg := &config.Context{
		Name:        c.Name,
		Connections: make([]config.MachineConnection, len(c.Machines)),
	}
	for i, m := range c.Machines {
		clusterCfg.Connections[i] = config.MachineConnection{
			TCP: &m.APIAddress,
		}
	}

	cfg.Contexts[c.Name] = clusterCfg
	cfg.CurrentContext = c.Name

	if err = cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

func (u *ConfigUpdater) RemoveCluster(name string) error {
	cfg, err := config.NewFromFile(u.path)
	if err != nil {
		return fmt.Errorf("read Uncloud config: %w", err)
	}

	if _, ok := cfg.Contexts[name]; !ok {
		return nil
	}

	delete(cfg.Contexts, name)

	if cfg.CurrentContext == name {
		cfg.CurrentContext = ""
	}
	if _, ok := cfg.Contexts["default"]; ok {
		cfg.CurrentContext = "default"
	}

	if err = cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
