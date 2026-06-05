package api

import (
	"io"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

// MachineFilter defines criteria to filter machines in ListMachines.
type MachineFilter struct {
	// Available filters machines that are not DOWN.
	Available bool
	// NamesOrIDs filters machines by their names or IDs.
	NamesOrIDs []string
}

type MachineExecOptions struct {
	Command     []string
	AttachStdin bool
	Tty         bool

	// Client-side only fields.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type MachineMembersList []*pb.MachineMember

func (m MachineMembersList) FindByNameOrID(nameOrID string) *pb.MachineMember {
	for _, machine := range m {
		if machine.Machine.Id == nameOrID || machine.Machine.Name == nameOrID {
			return machine
		}
	}

	return nil
}
