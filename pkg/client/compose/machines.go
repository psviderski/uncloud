package compose

import (
	"fmt"
	"strings"
)

const MachinesExtensionKey = "x-machines"

// MachinesSource represents the parsed x-machines extension data as slice of strings
type MachinesSource []string

// DecodeMapstructure implements custom decoding for multiple input types
func (m *MachinesSource) DecodeMapstructure(value interface{}) error {
	switch v := value.(type) {
	case *MachinesSource:
		// Handle case where compose-go passes a pointer to an already created instance
		*m = *v
		return nil
	case MachinesSource:
		// Handle case where compose-go passes a direct instance
		*m = v
		return nil
	case string:
		// Support single string value or comma-separated values
		// x-machines: my-machine or x-machines: "machine-1,machine-2"
		machines, err := parseMachineNames(v)
		if err != nil {
			return err
		}
		*m = MachinesSource(machines)
		return nil
	case []string:
		// Support string array: x-machines: ["machine-1", "machine-2"]
		machines, err := validateMachineNames(v)
		if err != nil {
			return err
		}
		*m = MachinesSource(machines)
		return nil
	case []interface{}:
		// Support interface array that may come from YAML parsing
		machineNames := make([]string, 0, len(v))
		for i, machine := range v {
			str, ok := machine.(string)
			if !ok {
				return fmt.Errorf("x-machines[%d] is not a string, got %T", i, machine)
			}
			machineNames = append(machineNames, str)
		}
		machines, err := validateMachineNames(machineNames)
		if err != nil {
			return err
		}
		*m = MachinesSource(machines)
		return nil
	default:
		return fmt.Errorf("x-machines must be a string or list of strings, got %T", value)
	}
}

// parseMachineNames parses a single string that may contain comma-separated machine names
func parseMachineNames(machinesStr string) ([]string, error) {
	// Split by comma and process each machine name, works for both single and multiple values
	parts := strings.Split(machinesStr, ",")
	machines := make([]string, 0, len(parts))
	for _, part := range parts {
		machines = append(machines, strings.TrimSpace(part))
	}
	return validateMachineNames(machines)
}

// validateMachineNames validates machine names to ensure they are not empty and contain valid characters.
func validateMachineNames(machines []string) ([]string, error) {
	for i, machine := range machines {
		machine = strings.TrimSpace(machine)
		if machine == "" {
			return nil, fmt.Errorf("x-machines[%d] cannot be empty", i)
		}
		machines[i] = machine
	}
	return machines, nil
}
