package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

type Config struct {
	CurrentContext string              `yaml:"current_context"`
	Contexts       map[string]*Context `yaml:"contexts"`

	// path is the file path config is read from.
	path string
}

func NewFromFile(path string) (*Config, error) {
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("check file permissions '%s': %w", path, err)
	}
	c := &Config{
		Contexts: map[string]*Context{},
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

func (c *Config) Path() string {
	return c.path
}

func (c *Config) Read() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return fmt.Errorf("read config file '%s': %w", c.path, err)
	}
	if err = yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("parse config file '%s': %s", c.path, yaml.FormatError(err, true, true))
	}

	return nil
}

func (c *Config) Save() error {
	dir, _ := filepath.Split(c.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config directory '%s': %w", dir, err)
	}

	f, err := os.OpenFile(c.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("write config file '%s': %w", c.path, err)
	}

	encoder := yaml.NewEncoder(f, yaml.Indent(2), yaml.IndentSequence(true))
	if err = encoder.Encode(c); err != nil {
		_ = f.Close()
		return fmt.Errorf("encode config file '%s': %w", c.path, err)
	}
	return f.Close()
}
