package corroservice

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"path/filepath"
)

const (
	LatestImage = "corrosion:latest"
)

type DockerService struct {
	Client  *client.Client
	Image   string
	Name    string
	DataDir string
	// TODO: uid/guid
}

func NewDockerService(cli *client.Client, image, name, dataDir string) *DockerService {
	return &DockerService{
		Client:  cli,
		Image:   image,
		Name:    name,
		DataDir: dataDir,
	}
}

func (s *DockerService) Start(ctx context.Context) error {
	_, err := s.Client.ContainerInspect(ctx, s.Name)
	if err != nil {
		if client.IsErrNotFound(err) {
			return s.startNewContainer(ctx)
		}
		return fmt.Errorf("inspect container %q: %w", s.Name, err)
	}
	// Container already exists, recreate it if its configuration has to be changed.
	// TODO: check config equal to the new one
	if err = s.Client.ContainerRemove(ctx, s.Name, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("remove container %q: %w", s.Name, err)
	}

	return s.startNewContainer(ctx)
}

func (s *DockerService) Restart(ctx context.Context) error {
	if err := s.Client.ContainerRestart(ctx, s.Name, container.StopOptions{}); err != nil {
		return fmt.Errorf("restart container %q: %w", s.Name, err)
	}
	return nil
}

func (s *DockerService) Running() bool {
	c, err := s.Client.ContainerInspect(context.Background(), s.Name)
	if err != nil {
		return false
	}

	return c.State.Running
}

func (s *DockerService) containerConfig() *container.Config {
	return &container.Config{
		Image: s.Image,
		Env: []string{
			fmt.Sprintf("CONFIG_PATH=%s", filepath.Join(s.DataDir, "config.toml")),
		},
	}
}

func (s *DockerService) hostConfig() *container.HostConfig {
	return &container.HostConfig{
		NetworkMode: network.NetworkHost,
	}
}

func (s *DockerService) startNewContainer(ctx context.Context) error {
	_, err := s.Client.ContainerCreate(ctx, s.containerConfig(), s.hostConfig(), nil, nil, s.Name)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	if err = s.Client.ContainerStart(ctx, s.Name, container.StartOptions{}); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	return nil
}
