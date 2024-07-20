package machine

import (
	"fmt"
	"path/filepath"
	"uncloud/internal/machine/daemon"
)

func Install(dataDir string, cfg daemon.Config) error {
	err := cfg.Write(filepath.Join(dataDir, daemon.MachineConfigPath))
	if err == nil {
		fmt.Println("Machine config created.")
	}
	return err
}
