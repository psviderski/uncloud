package corroservice

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/containerd/errdefs"
	systemd "github.com/coreos/go-systemd/daemon"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/psviderski/uncloud/pkg/api"
)

const (
	// Image is the Corrosion image pinned to the uncloudd version.
	Image = "ghcr.io/unlabs-dev/corrosion:2026.6.15"
	// ContainerName is the name of the managed Corrosion container.
	ContainerName = "uncloud-corrosion"

	// systemdStartTimeoutExtension is the duration of each systemd service start timeout extension that prevents
	// a slow image pull from exceeding the start timeout (TimeoutStartSec). Extensions are a no-op if Corrosion
	// is not starting during a systemd service startup.
	systemdStartTimeoutExtension = 30 * time.Second
	// systemdStartTimeoutExtendMax caps the total duration the start timeout can be extended for.
	systemdStartTimeoutExtendMax = 5 * time.Minute
)

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

	corro, err := NewAPIClient(s.DataDir)
	if err != nil {
		return err
	}
	slog.Debug("Waiting for corrosion service to be ready.")
	if err = WaitReady(ctx, corro); err != nil {
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

		// The pull on the first daemon start may take longer than the systemd service start timeout (TimeoutStartSec)
		// on a slow connection. Keep extending the timeout while the pull is in progress so systemd doesn't kill
		// the daemon before it reports readiness. This is a no-op if not running under systemd.
		stopExtending := extendSystemdStartTimeout(ctx)
		// Wait for pull to complete.
		_, err = io.Copy(io.Discard, respBody)
		stopExtending()
		if err != nil {
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

// extendSystemdStartTimeout periodically asks systemd to extend the service start timeout, for up to
// systemdStartTimeoutExtendMax. The returned function stops the extensions. It is a no-op when the service
// is not running under systemd (NOTIFY_SOCKET is not set).
func extendSystemdStartTimeout(ctx context.Context) (stop context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		// Send extensions more frequently than they expire so a single missed tick doesn't time out the start.
		ticker := time.NewTicker(systemdStartTimeoutExtension / 3)
		defer ticker.Stop()
		deadline := time.Now().Add(systemdStartTimeoutExtendMax)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Now().After(deadline) {
					slog.Warn("Stopped extending the systemd service start timeout: corrosion service is taking "+
						"too long to start.", "timeout", systemdStartTimeoutExtendMax)
					return
				}
				msg := fmt.Sprintf("EXTEND_TIMEOUT_USEC=%d", systemdStartTimeoutExtension.Microseconds())
				if _, err := systemd.SdNotify(false, msg); err != nil {
					slog.Warn("Failed to extend the systemd service start timeout when starting corrosion service.",
						"err", err)
					return
				}
				slog.Info("Extended systemd service start timeout while corrosion service is starting.",
					"duration", systemdStartTimeoutExtension)
			}
		}
	}()

	return cancel
}
