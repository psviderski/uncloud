package daemon

import (
	"errors"
	"fmt"
	"path/filepath"
)

func Run(dataDir string) error {
	cfg, err := ReadConfig(filepath.Join(dataDir, MachineConfigPath))
	if err != nil {
		return err
	}
	fmt.Println("Config", cfg)

	return errors.New("Running daemon...")
}
