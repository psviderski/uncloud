package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultDataDir    = "/var/lib/uncloud"
	MachineConfigPath = "machine.json"
)

type Config struct {
	UncloudID     string
	UncloudSecret string
	// IPv4 network in CIDR format to use for the machine network.
	Network string
}

func ReadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file %q: %w", path, err)
	}
	var config Config
	if err = json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("parse config file %q: %w", path, err)
	}
	return config, nil
}

func (c *Config) Write(path string) error {
	dir, _ := filepath.Split(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config directory %q: %w", dir, err)
	}

	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}
