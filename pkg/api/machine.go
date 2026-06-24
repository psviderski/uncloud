package api

import (
	"net/netip"
	"strings"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

// MachineFilter defines criteria to filter machines in ListMachines.
type MachineFilter struct {
	// Available filters machines that are not DOWN.
	Available bool
	// NamesOrIDs filters machines by their names or IDs.
	NamesOrIDs []string
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

// ToNative returns the Go native representation of the protobuf machine members.
func (m MachineMembersList) ToNative() []MachineMember {
	infos := make([]MachineMember, len(m))
	for i, member := range m {
		infos[i] = machineMemberFromProto(member)
	}
	return infos
}

// MachineMember is the JSON-serializable view of a machine member.
type MachineMember struct {
	ID       string
	Name     string
	State    string
	Network  MachineNetwork
	PublicIP netip.Addr

	DaemonVersion string
	DockerVersion string
	Hostname      string
	Arch          string
	OSPrettyName  string
	KernelVersion string
}

// MachineNetwork describes a machine's WireGuard network configuration.
type MachineNetwork struct {
	Subnet       netip.Prefix
	ManagementIP netip.Addr
	Endpoints    []netip.AddrPort
	// PublicKey is the WireGuard public key.
	PublicKey []byte
}

func machineMemberFromProto(pbMember *pb.MachineMember) MachineMember {
	m := pbMember.Machine
	member := MachineMember{
		ID:            m.Id,
		Name:          m.Name,
		State:         capitalise(pbMember.State.String()),
		DaemonVersion: m.DaemonVersion,
		DockerVersion: m.DockerVersion,
		Hostname:      m.Hostname,
		Arch:          m.Arch,
		OSPrettyName:  m.OsPrettyName,
		KernelVersion: m.KernelVersion,
	}

	if m.PublicIp != nil {
		if ip, err := m.PublicIp.ToAddr(); err == nil {
			member.PublicIP = ip
		}
	}

	if m.Network != nil {
		network := MachineNetwork{
			PublicKey: m.Network.PublicKey,
		}
		if m.Network.Subnet != nil {
			if subnet, err := m.Network.Subnet.ToPrefix(); err == nil {
				network.Subnet = subnet
			}
		}
		if m.Network.ManagementIp != nil {
			if ip, err := m.Network.ManagementIp.ToAddr(); err == nil {
				network.ManagementIP = ip
			}
		}
		endpoints := make([]netip.AddrPort, 0, len(m.Network.Endpoints))
		for _, ep := range m.Network.Endpoints {
			if addrPort, err := ep.ToAddrPort(); err == nil {
				endpoints = append(endpoints, addrPort)
			}
		}
		network.Endpoints = endpoints
		member.Network = network
	}

	return member
}

func capitalise(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}
