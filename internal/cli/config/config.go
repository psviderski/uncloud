package config

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"os"
	"path/filepath"
)

type Config struct {
	Clusters       map[string]*Cluster `toml:"clusters"`
	CurrentCluster string              `toml:"current_cluster"`

	// path is the file path config is read from.
	path string
}

func NewFromFile(path string) (*Config, error) {
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("check file permissions %q: %w", path, err)
	}
	c := &Config{
		Clusters: map[string]*Cluster{},
		path:     path,
	}
	if os.IsNotExist(err) {
		return c, nil
	}

	if err = c.Read(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) Read() error {
	_, err := toml.DecodeFile(c.path, c)
	if err != nil {
		return fmt.Errorf("read config file %q: %w", c.path, err)
	}
	return nil
}

func (c *Config) Save() error {
	dir, _ := filepath.Split(c.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config directory %q: %w", dir, err)
	}

	f, err := os.OpenFile(c.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("write config file %q: %w", c.path, err)
	}

	encoder := toml.NewEncoder(f)
	encoder.Indent = ""
	if err = encoder.Encode(c); err != nil {
		_ = f.Close()
		return fmt.Errorf("encode config file %q: %w", c.path, err)
	}
	return f.Close()
}
