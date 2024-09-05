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
	StateFileName  = "machine.json"
	APIPort        = 51000
)

// State defines the machine-specific configuration within a cluster. It encapsulates essential identifiers
// and settings required to establish an overlay network and operate as a member of a cluster.
type State struct {
	// ID uniquely identifies this machine in the cluster.
	ID string
	// Name provides a human-readable identifier for the machine.
	Name string
	// Network specifies the network configuration for this machine.
	Network *network.Config

	// path is the file path config is read from and saved to.
	path string
}

// StatePath returns the path to the machine state file within the given data directory.
func StatePath(dataDir string) string {
	return filepath.Join(dataDir, StateFileName)
}

// ParseState reads and decodes a state from the file at the given path.
func ParseState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	var config State
	if err = json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}

	if config.Network == nil {
		return nil, fmt.Errorf("missing network configuration in config file %q", path)
	}
	return &config, nil
}

// SetPath sets the file path the state can be saved to.
func (c *State) SetPath(path string) {
	c.path = path
}

// Encode returns the JSON encoded state data.
func (c *State) Encode() ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return data, nil
}

// Save writes the state data to the file at the given path.
func (c *State) Save() error {
	if c.path == "" {
		return fmt.Errorf("config path not set")
	}
	dir, _ := filepath.Split(c.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config directory %q: %w", dir, err)
	}

	data, err := c.Encode()
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0600)
}

// NewID generates a new unique machine ID.
func NewID() (string, error) {
	return secret.NewID()
}

// NewRandomName generates a random machine name in the format "machine-xxxx".
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

// NewBootstrapConfig returns a new machine configuration that should be applied to the first machine in a cluster.
func NewBootstrapConfig(name string, subnet netip.Prefix, peers ...network.PeerConfig) (*State, error) {
	mid, err := NewID()
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

	return &State{
		ID:   mid,
		Name: name,
		Network: &network.Config{
			Subnet:       subnet,
			ManagementIP: network.ManagementIP(pubKey),
			PrivateKey:   privKey,
			PublicKey:    pubKey,
			Peers:        peers,
		},
	}, nil
}
