package machine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/psviderski/uncloud/internal/machine/network"
)

const (
	DefaultDataDir = "/var/lib/uncloud"
	StateFileName  = "machine.json"
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
	// MinStoreDBVersion is the latest database version of one of the existing cluster machines at the time this machine
	// joined the cluster. The machine should sync to at least this version before starting any cluster operations.
	MinStoreDBVersion int64 `json:",omitempty"`

	// path is the file path config is read from and saved to.
	path string
	// mu protects the state from concurrent reads and writes.
	mu sync.RWMutex
}

// StatePath returns the path to the machine state file within the given data directory.
func StatePath(dataDir string) string {
	return filepath.Join(dataDir, StateFileName)
}

// ParseState reads and decodes a state from the file at the given path.
func ParseState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}
	state := State{path: path}
	if err = json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse state file %q: %w", path, err)
	}
	if state.Network == nil {
		return nil, fmt.Errorf("missing network configuration in state file %q", path)
	}
	return &state, nil
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

// Save atomically writes the state data to the file at the configured path.
func (c *State) Save() error {
	if c.path == "" {
		return fmt.Errorf("state path not set")
	}
	dir, _ := filepath.Split(c.path)
	if err := os.MkdirAll(dir, 0o711); err != nil {
		return fmt.Errorf("create state directory %q: %w", dir, err)
	}

	data, err := c.Encode()
	if err != nil {
		return err
	}

	// Write to a temporary file and rename for atomic save. CreateTemp creates a file with mode 0o600.
	tmpFile, err := os.CreateTemp(dir, ".machine.json.*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on error.
	defer func() {
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	if _, err = tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	// Without fsync, the data may only be in the OS buffer cache. If the system crashes before the OS flushes
	// it to disk, the file could be empty or corrupted even after rename.
	if err = tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err = os.Rename(tmpPath, c.path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
