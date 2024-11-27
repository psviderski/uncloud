package corroservice

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"io"
	"log/slog"
	"path/filepath"
	"time"
)

const (
	LatestImage = "ghcr.io/psviderski/corrosion:latest"
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
		Cmd:   []string{"corrosion", "agent", "-c", filepath.Join(s.DataDir, "config.toml")},
	}
}

func (s *DockerService) hostConfig() *container.HostConfig {
	return &container.HostConfig{
		NetworkMode: network.NetworkHost,
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyAlways,
		},
	}
}

func (s *DockerService) startNewContainer(ctx context.Context) error {
	_, err := s.Client.ContainerCreate(ctx, s.containerConfig(), s.hostConfig(), nil, nil, s.Name)
	if err != nil {
		if !client.IsErrNotFound(err) {
			return fmt.Errorf("create container: %w", err)
		}

		slog.Info("Pulling Docker image for corrosion service.", "image", s.Image)
		start := time.Now()

		respBody, err := s.Client.ImagePull(ctx, s.Image, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("pull image: %w", err)
		}
		defer respBody.Close()

		// Wait for pull to complete.
		if _, err := io.Copy(io.Discard, respBody); err != nil {
			return fmt.Errorf("read pull response: %w", err)
		}

		slog.Info("Docker image pulled.", "image", s.Image, "duration", time.Since(start).String())

		// Create container again after image pull.
		if _, err = s.Client.ContainerCreate(ctx, s.containerConfig(), s.hostConfig(), nil, nil, s.Name); err != nil {
			return fmt.Errorf("create container: %w", err)
		}
	}

	if err = s.Client.ContainerStart(ctx, s.Name, container.StartOptions{}); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	return nil
}
