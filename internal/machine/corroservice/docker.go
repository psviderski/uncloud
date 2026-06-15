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
	"github.com/psviderski/uncloud/pkg/api"
)

// Image is the Corrosion image pinned to the uncloudd version.
const Image = "ghcr.io/unlabs-dev/corrosion:2026.6.15"

type DockerService struct {
	Client *client.Client
	Image  string
	Name   string
	// DataDir holds the corrosion config, schema, and db files.
	DataDir string
	// RunDir holds ephemeral runtime state like the admin socket.
	RunDir string
	User   string
}

func (s *DockerService) Start(ctx context.Context) error {
	c, err := s.Client.ContainerInspect(ctx, s.Name)
	switch {
	case errdefs.IsNotFound(err):
		if err = s.createAndStart(ctx); err != nil {
			return err
		}
	case err != nil:
		return fmt.Errorf("inspect container '%s': %w", s.Name, err)
	case c.Config.Image != s.Image:
		slog.Info("Corrosion container image needs update, recreating container.",
			"name", s.Name, "current_image", c.Config.Image, "new_image", s.Image)

		// Gracefully stop the container before removing it.
		if err = s.Client.ContainerStop(ctx, s.Name, container.StopOptions{}); err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("stop container '%s': %w", s.Name, err)
		}
		if err = s.Client.ContainerRemove(ctx, s.Name, container.RemoveOptions{
			// Remove anonymous volumes created by the container.
			RemoveVolumes: true,
		}); err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("remove container '%s': %w", s.Name, err)
		}
		if err = s.createAndStart(ctx); err != nil {
			return err
		}
	case !c.State.Running:
		slog.Debug("Starting existing Corrosion container.", "name", s.Name)
		if err = s.Client.ContainerStart(ctx, s.Name, container.StartOptions{}); err != nil {
			return fmt.Errorf("start container '%s': %w", s.Name, err)
		}
	}

	slog.Debug("Waiting for corrosion service to be ready.")
	if err = WaitReady(ctx, s.DataDir); err != nil {
		return err
	}
	slog.Debug("Corrosion service is ready.")
	return nil
}

// Stop stops the Corrosion container without removing it. The container is kept so that
// the next Start can start it instead of pulling and recreating.
func (s *DockerService) Stop(ctx context.Context) error {
	if err := s.Client.ContainerStop(ctx, s.Name, container.StopOptions{}); err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("stop container '%s': %w", s.Name, err)
	}
	slog.Debug("Corrosion container stopped.", "name", s.Name)
	return nil
}

func (s *DockerService) Restart(ctx context.Context) error {
	if err := s.Client.ContainerRestart(ctx, s.Name, container.StopOptions{}); err != nil {
		return fmt.Errorf("restart container '%s': %w", s.Name, err)
	}
	return nil
}

// Cleanup gracefully stops and removes the Corrosion container.
func (s *DockerService) Cleanup(ctx context.Context) error {
	if err := s.Client.ContainerStop(ctx, s.Name, container.StopOptions{}); err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("stop container '%s': %w", s.Name, err)
	}
	if err := s.Client.ContainerRemove(ctx, s.Name, container.RemoveOptions{
		RemoveVolumes: true,
	}); err != nil {
		return fmt.Errorf("remove container '%s': %w", s.Name, err)
	}
	slog.Debug("Corrosion container removed.", "name", s.Name)
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
		Labels: map[string]string{
			api.LabelDaemonManaged: "",
		},
	}
}

func (s *DockerService) hostConfig() *container.HostConfig {
	return &container.HostConfig{
		NetworkMode: network.NetworkHost,
		// Use unless-stopped so uncloudd-initiated stops are honoured.
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
		LogConfig: container.LogConfig{
			Type: "local",
		},
		Mounts: []mount.Mount{
			// Bind mount the data and runtime directories at the same paths inside the container
			// to simplify path handling.
			{
				Type:   mount.TypeBind,
				Source: s.DataDir,
				Target: s.DataDir,
			},
			{
				Type:   mount.TypeBind,
				Source: s.RunDir,
				Target: s.RunDir,
			},
		},
	}
}

func (s *DockerService) createAndStart(ctx context.Context) error {
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
