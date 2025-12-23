package ucind

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/go-connections/nat"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/connector"
)

const (
	DefaultImage   = "ghcr.io/psviderski/ucind:latest"
	UncloudAPIPort = 51000

	MachineNameLabel = "ucind.machine.name"
)

type Machine struct {
	ClusterName   string
	ContainerName string
	ID            string
	Name          string
	APIAddress    netip.AddrPort
}

func (m *Machine) Connect(ctx context.Context) (*client.Client, error) {
	return client.New(ctx, connector.NewTCPConnector(m.APIAddress))
}

type CreateMachineOptions struct {
	Name  string
	Image string
	// Ports to forward from the machine to the host.
	PortMap nat.PortMap
}

func (p *Provisioner) CreateMachine(ctx context.Context, clusterName string, opts CreateMachineOptions) (Machine, error) {
	var m Machine

	machineName := opts.Name
	if machineName == "" {
		var err error
		if machineName, err = randomMachineName(); err != nil {
			return m, fmt.Errorf("generate random machine name: %w", err)
		}
	}
	containerName := clusterName + "-" + machineName

	img := DefaultImage
	if opts.Image != "" {
		img = opts.Image
	}

	apiPort := nat.Port(fmt.Sprintf("%d/tcp", UncloudAPIPort))
	config := &container.Config{
		Image: img,
		Labels: map[string]string{
			ClusterNameLabel: clusterName,
			MachineNameLabel: machineName,
			ManagedLabel:     "",
		},
		ExposedPorts: nat.PortSet{
			apiPort: struct{}{},
		},
	}
	hostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode(clusterName),
		PortBindings: nat.PortMap{
			apiPort: []nat.PortBinding{
				{
					HostIP: "127.0.0.1",
					// Host port is a random available port.
				},
			},
		},
		Privileged: true,
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyAlways,
		},
	}
	// Forward ports, if requested
	if opts.PortMap != nil {
		for port, bindings := range opts.PortMap {
			if len(bindings) == 0 {
				continue
			}
			config.ExposedPorts[port] = struct{}{}
			hostConfig.PortBindings[port] = bindings
		}
	}

	if _, err := p.createContainerWithImagePull(ctx, containerName, config, hostConfig); err != nil {
		return m, err
	}
	if err := p.dockerCli.ContainerStart(ctx, containerName, container.StartOptions{}); err != nil {
		return m, fmt.Errorf("start Docker container: %w", err)
	}

	apiPortBindings, err := p.waitPortPublished(ctx, containerName, apiPort)
	if err != nil {
		return m, fmt.Errorf("wait for machine API port '%s' to be published: %w", apiPort, err)
	}
	apiAddr, err := netip.ParseAddrPort(net.JoinHostPort(apiPortBindings[0].HostIP, apiPortBindings[0].HostPort))
	if err != nil {
		return m, fmt.Errorf("parse machine API port binding: %w", err)
	}

	m = Machine{
		ClusterName:   clusterName,
		ContainerName: containerName,
		Name:          machineName,
		APIAddress:    apiAddr,
	}
	return m, nil
}

// createContainerWithImagePull creates a Docker container. If the image is missing, it pulls the image first.
func (p *Provisioner) createContainerWithImagePull(
	ctx context.Context, name string, config *container.Config, hostConfig *container.HostConfig,
) (container.CreateResponse, error) {
	var resp container.CreateResponse

	_, err := p.dockerCli.ContainerCreate(ctx, config, hostConfig, nil, nil, name)
	if err == nil {
		return resp, nil
	}

	if !errdefs.IsNotFound(err) {
		return resp, fmt.Errorf("create Docker container: %w", err)
	}

	respBody, err := p.dockerCli.ImagePull(ctx, config.Image, image.PullOptions{})
	if err != nil {
		return resp, fmt.Errorf("pull Docker image: %w", err)
	}
	defer respBody.Close()

	// Wait for pull to complete.
	if _, err = io.Copy(io.Discard, respBody); err != nil {
		return resp, fmt.Errorf("read Docker pull response: %w", err)
	}

	// Create container again after image pull.
	if resp, err = p.dockerCli.ContainerCreate(ctx, config, hostConfig, nil, nil, name); err != nil {
		return resp, fmt.Errorf("create Docker container: %w", err)
	}

	return resp, nil
}

// waitPortPublished waits for a Docker container port to be published on the host which happens asynchronously.
func (p *Provisioner) waitPortPublished(ctx context.Context, containerID string, port nat.Port) ([]nat.PortBinding, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for {
		c, err := p.dockerCli.ContainerInspect(ctx, containerID)
		if err != nil {
			return nil, fmt.Errorf("inspect container: %w", err)
		}

		binding, ok := c.NetworkSettings.Ports[port]
		if ok && len(binding) > 0 {
			return binding, nil
		}

		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, errors.New("timeout")
			}
			return nil, ctx.Err()
		}
	}
}

func randomMachineName() (string, error) {
	suffix, err := secret.RandomAlphaNumeric(4)
	if err != nil {
		return "", fmt.Errorf("generate random suffix: %w", err)
	}
	return "machine-" + suffix, nil
}
