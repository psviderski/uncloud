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

	if _, ok := cfg.Clusters[c.Name]; ok {
		return fmt.Errorf("cluster '%s' already exists", c.Name)
	}

	clusterCfg := &config.Cluster{
		Name:        c.Name,
		Connections: make([]config.MachineConnection, len(c.Machines)),
	}
	for i, m := range c.Machines {
		clusterCfg.Connections[i] = config.MachineConnection{
			TCP: m.APIAddress,
		}
	}

	cfg.Clusters[c.Name] = clusterCfg
	cfg.CurrentCluster = c.Name

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

	if _, ok := cfg.Clusters[name]; !ok {
		return nil
	}

	delete(cfg.Clusters, name)

	if cfg.CurrentCluster == name {
		cfg.CurrentCluster = ""
	}
	if _, ok := cfg.Clusters["default"]; ok {
		cfg.CurrentCluster = "default"
	}

	if err = cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
