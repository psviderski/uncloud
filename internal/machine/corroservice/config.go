package corroservice

import (
	"bytes"
	"fmt"
	"github.com/BurntSushi/toml"
	"net/netip"
	"os"
	"uncloud/internal/fs"
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
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		return err
	}
	if err := fs.Chown(path, owner); err != nil {
		return err
	}
	return nil
}
