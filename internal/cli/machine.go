package cli

import "uncloud/internal/cli/config"

type Machine struct {
	connConfig config.MachineConnection
}

func NewMachine(connConfig config.MachineConnection) *Machine {
	return &Machine{connConfig: connConfig}
}
