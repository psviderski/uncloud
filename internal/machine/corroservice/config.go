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
	Addr netip.AddrPort `toml:"addr"`
	// MaxMTU is the upper bound for QUIC's MTU, preventing automatic MTU discovery. It is the effective MTU:
	// the gossip network interface's MTU minus the IP and UDP header sizes. Must be >= 1200. Omitted when zero.
	MaxMTU    uint32   `toml:"max_mtu,omitzero"`
	Bootstrap []string `toml:"bootstrap"`
	Plaintext bool     `toml:"plaintext"`
}

type APIConfig struct {
	Addr  netip.AddrPort `toml:"addr"`
	Authz APIAuthzConfig `toml:"authz"`
}

type APIAuthzConfig struct {
	BearerToken string `toml:"bearer-token"`
}

type AdminConfig struct {
	Path string `toml:"path"`
}

// Marshal encodes the config as TOML.
func (c *Config) Marshal() ([]byte, error) {
	var data bytes.Buffer
	encoder := toml.NewEncoder(&data)
	encoder.Indent = "" // Disable indentation.
	if err := encoder.Encode(c); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	return data.Bytes(), nil
}

func (c *Config) Write(path, owner string) error {
	data, err := c.Marshal()
	if err != nil {
		return err
	}
	if err = os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return fs.Chown(path, owner, owner)
}

// MkDir creates a data or runtime directory for the Corrosion service with 0700 permissions and the specified owner.
func MkDir(dir, owner string) error {
	parent, _ := filepath.Split(dir)
	// Use 0711 for parent directories to allow `owner` to access its nested directory.
	if err := os.MkdirAll(parent, 0o711); err != nil {
		return fmt.Errorf("create directory '%s': %w", parent, err)
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
