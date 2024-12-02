package service

import (
	"encoding/json"
	"fmt"
	"github.com/docker/docker/api/types"
	"uncloud/internal/machine/api/pb"
)

const (
	ModeReplicated = "replicated"
	ModeGlobal     = "global"
)

type Service struct {
	ID         string
	Name       string
	Containers []Container
}

type Container struct {
	MachineID string
	Container types.Container
}

func FromProto(s *pb.Service) (Service, error) {
	var err error
	containers := make([]Container, len(s.Containers))
	for i, c := range s.Containers {
		containers[i], err = containerFromProto(c)
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

func containerFromProto(c *pb.Service_Container) (Container, error) {
	var dockerCtr types.Container
	if err := json.Unmarshal(c.Container, &dockerCtr); err != nil {
		return Container{}, fmt.Errorf("unmarshal container: %w", err)
	}

	return Container{
		MachineID: c.MachineId,
		Container: dockerCtr,
	}, nil
}
