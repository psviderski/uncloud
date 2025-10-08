package corroservice

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

const (
	LatestImage = "ghcr.io/psviderski/corrosion:latest"
)

type DockerService struct {
	Client  *client.Client
	Image   string
	Name    string
	DataDir string
	User    string
}

func (s *DockerService) Start(ctx context.Context) error {
	_, err := s.Client.ContainerInspect(ctx, s.Name)
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("inspect container %q: %w", s.Name, err)
		}
		if err = s.startNewContainer(ctx); err != nil {
			return err
		}
	} else {
		// Container already exists.
		// TODO: recreate only if the container configuration has to be changed.
		if err = s.Client.ContainerRemove(ctx, s.Name, container.RemoveOptions{Force: true}); err != nil {
			return fmt.Errorf("remove container %q: %w", s.Name, err)
		}
		if err = s.startNewContainer(ctx); err != nil {
			return err
		}
	}

	slog.Debug("Waiting for corrosion service to be ready.")
	if err = WaitReady(ctx, s.DataDir); err != nil {
		return err
	}
	slog.Debug("Corrosion service is ready.")

	return nil
}

func (s *DockerService) Stop(ctx context.Context) error {
	if err := s.Client.ContainerStop(ctx, s.Name, container.StopOptions{}); err != nil {
		return fmt.Errorf("stop container %q: %w", s.Name, err)
	}
	slog.Debug("Corrosion Docker container stopped.", "name", s.Name)

	if err := s.Client.ContainerRemove(ctx, s.Name, container.RemoveOptions{}); err != nil {
		return fmt.Errorf("remove container %q: %w", s.Name, err)
	}
	slog.Debug("Corrosion Docker container removed.", "name", s.Name)

	return nil
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
		User:  s.User,
	}
}

func (s *DockerService) hostConfig() *container.HostConfig {
	return &container.HostConfig{
		NetworkMode: network.NetworkHost,
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyAlways,
		},
		Mounts: []mount.Mount{
			// Bind mount the data directory at the same path inside the container to simplify path handling.
			{
				Type:   mount.TypeBind,
				Source: s.DataDir,
				Target: s.DataDir,
			},
		},
	}
}

func (s *DockerService) startNewContainer(ctx context.Context) error {
	_, err := s.Client.ContainerCreate(ctx, s.containerConfig(), s.hostConfig(), nil, nil, s.Name)
	if err != nil {
		if !errdefs.IsNotFound(err) {
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
