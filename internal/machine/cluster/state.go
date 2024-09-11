package cluster

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"os"
	"path/filepath"
	"uncloud/internal/machine/api/pb"
)

const StateFile = "cluster.pb"

type State struct {
	State *pb.State
	path  string
}

func StatePath(dataDir string) string {
	return filepath.Join(dataDir, StateFile)
}

func NewState(path string) *State {
	return &State{
		State: &pb.State{
			Machines:  make(map[string]*pb.MachineInfo),
			Endpoints: make(map[string]*pb.MachineEndpoints),
		},
		path: path,
	}
}

func (s *State) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read state file %q: %w", s.path, err)
	}
	if err = proto.Unmarshal(data, s.State); err != nil {
		return fmt.Errorf("parse state file %q: %w", s.path, err)
	}
	if s.State.Machines == nil {
		s.State.Machines = make(map[string]*pb.MachineInfo)
	}
	if s.State.Endpoints == nil {
		s.State.Endpoints = make(map[string]*pb.MachineEndpoints)
	}
	return nil
}

func (s *State) Save() error {
	dir, _ := filepath.Split(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create state directory %q: %w", dir, err)
	}

	data, err := proto.Marshal(s.State)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return os.WriteFile(s.path, data, 0600)
}
