package machine

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/netip"
	"os"
	"path/filepath"
	"uncloud/internal/machine/network"
	"uncloud/internal/secret"
)

const (
	DefaultDataDir = "/var/lib/uncloud"
	ConfigFileName = "machine.json"
)

// Config defines the machine-specific configuration within a cluster for the Uncloud daemon. It encapsulates
// essential identifiers and settings required to establish an overlay network and join the cluster.
type Config struct {
	// ID uniquely identifies this machine in the cluster.
	ID string
	// Name provides a human-readable identifier for the machine.
	Name string
	// Network specifies the network configuration for this machine.
	Network *network.Config
}

// NewRandomName returns a random machine name in the format "machine-xxxx".
func NewRandomName() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	suffix := make([]byte, 4)
	for i := range suffix {
		randIdx, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("get random number: %w", err)
		}
		suffix[i] = charset[randIdx.Int64()]
	}
	return "machine-" + string(suffix), nil
}

// ConfigPath returns the path to the machine configuration file within the given data directory.
func ConfigPath(dataDir string) string {
	return filepath.Join(dataDir, ConfigFileName)
}

// NewBootstrapConfig returns a new machine configuration that should be applied to the first machine in a cluster.
func NewBootstrapConfig(name string, subnet netip.Prefix) (*Config, error) {
	mid, err := secret.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate machine ID: %w", err)
	}
	if name == "" {
		name, err = NewRandomName()
		if err != nil {
			return nil, fmt.Errorf("generate machine name: %w", err)
		}
	}
	privKey, pubKey, err := network.NewMachineKeys()
	if err != nil {
		return nil, fmt.Errorf("generate machine keys: %w", err)
	}
	if subnet == (netip.Prefix{}) {
		// Use the first /24 subnet in the default network.
		subnet = netip.PrefixFrom(network.DefaultNetwork.Addr(), network.DefaultSubnetBits)
	}

	return &Config{
		ID:   mid,
		Name: name,
		Network: &network.Config{
			Subnet:     subnet,
			PrivateKey: privKey,
			PublicKey:  pubKey,
		},
	}, nil
}

// ParseConfig reads and decodes a config from the file at the given path.
func ParseConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}
	var config Config
	if err = json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}
	return &config, nil
}

// Encode returns the JSON encoded config data.
func (c *Config) Encode() ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return data, nil
}

// Save writes the config data to the file at the given path.
func (c *Config) Save(path string) error {
	dir, _ := filepath.Split(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config directory %q: %w", dir, err)
	}

	data, err := c.Encode()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
