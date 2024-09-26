package corrosion

import (
	"bytes"
	"fmt"
	"github.com/BurntSushi/toml"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
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

	if owner != "" {
		usr, err := user.Lookup(owner)
		if err != nil {
			return fmt.Errorf("lookup user %q: %w", owner, err)
		}
		uid, err := strconv.Atoi(usr.Uid)
		if err != nil {
			return fmt.Errorf("parse %q user ID (UID) %q: %w", owner, usr.Uid, err)
		}
		gid, err := strconv.Atoi(usr.Gid)
		if err != nil {
			return fmt.Errorf("parse %q user group ID (GID) %q: %w", owner, usr.Gid, err)
		}
		if err = os.Chown(path, uid, gid); err != nil {
			return fmt.Errorf("chown %q: %w", path, err)
		}
	}
	return nil
}

func MkDataDir(dir, owner string) error {
	parent, _ := filepath.Split(dir)
	// Use 0711 for parent directories to allow `owner` to access its nested data directory.
	if err := os.MkdirAll(parent, 0711); err != nil {
		return fmt.Errorf("create directory %q: %w", parent, err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}

	if owner != "" {
		usr, err := user.Lookup(owner)
		if err != nil {
			return fmt.Errorf("lookup user %q: %w", owner, err)
		}
		uid, err := strconv.Atoi(usr.Uid)
		if err != nil {
			return fmt.Errorf("parse %q user ID (UID) %q: %w", owner, usr.Uid, err)
		}
		gid, err := strconv.Atoi(usr.Gid)
		if err != nil {
			return fmt.Errorf("parse %q user group ID (GID) %q: %w", owner, usr.Gid, err)
		}
		if err = os.Chown(dir, uid, gid); err != nil {
			return fmt.Errorf("chown %q: %w", dir, err)
		}
	}
	return nil
}
