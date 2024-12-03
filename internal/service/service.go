package service

import (
	"encoding/json"
	"fmt"
	"uncloud/internal/machine/api/pb"
)

const (
	ModeReplicated = "replicated"
	ModeGlobal     = "global"
)

type Service struct {
	ID         string
	Name       string
	Containers []MachineContainer
}

type MachineContainer struct {
	MachineID string
	Container Container
}

func FromProto(s *pb.Service) (Service, error) {
	var err error
	containers := make([]MachineContainer, len(s.Containers))
	for i, sc := range s.Containers {
		containers[i], err = machineContainerFromProto(sc)
		if err != nil {
			return Service{}, err
		}
	}

	return Service{
		ID:         s.Id,
		Name:       s.Name,
		Containers: containers,
	}, nil
}

func machineContainerFromProto(sc *pb.Service_Container) (MachineContainer, error) {
	var c Container
	if err := json.Unmarshal(sc.Container, &c); err != nil {
		return MachineContainer{}, fmt.Errorf("unmarshal container: %w", err)
	}

	return MachineContainer{
		MachineID: sc.MachineId,
		Container: c,
	}, nil
}
