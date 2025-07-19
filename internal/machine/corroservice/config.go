package corroservice

import (
	"bytes"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/psviderski/uncloud/internal/fs"
)

const (
	DefaultUser       = "uncloud"
	DefaultGossipPort = 51001
	DefaultAPIPort    = 51002
)

// Config represents the Corrosion config.
type Config struct {
	DB     DBConfig     `toml:"db"`
	Gossip GossipConfig `toml:"gossip"`
	API    APIConfig    `toml:"api"`
	Admin  AdminConfig  `toml:"admin"`
}

type DBConfig struct {
	Path        string   `toml:"path"`
	SchemaPaths []string `toml:"schema_paths"`
}

type GossipConfig struct {
	Addr      netip.AddrPort `toml:"addr"`
	Bootstrap []string       `toml:"bootstrap"`
	Plaintext bool           `toml:"plaintext"`
}

type APIConfig struct {
	Addr netip.AddrPort `toml:"addr"`
}

type AdminConfig struct {
	Path string `toml:"path"`
}

func (c *Config) Write(path, owner string) error {
	var data bytes.Buffer
	encoder := toml.NewEncoder(&data)
	encoder.Indent = "" // Disable indentation.
	if err := encoder.Encode(c); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		return err
	}
	if err := fs.Chown(path, owner, owner); err != nil {
		return err
	}
	return nil
}

func MkDataDir(dir, owner string) error {
	parent, _ := filepath.Split(dir)
	// Use 0711 for parent directories to allow `owner` to access its nested data directory.
	if err := os.MkdirAll(parent, 0o711); err != nil {
		return fmt.Errorf("create directory %q: %w", parent, err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}

	if owner != "" {
		if err := fs.Chown(dir, owner, owner); err != nil {
			return err
		}
	}
	return nil
}
