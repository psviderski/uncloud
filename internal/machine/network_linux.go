//go:build linux

package machine

import (
	"context"
	"fmt"
	dnetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/libnetwork/iptables"
	"log/slog"
	"time"
	"uncloud/internal/machine/network"
)

// setupDockerNetwork creates the Docker bridge network DockerNetworkName with the machine subnet and configures
// iptables to allow WireGuard network to access containers.
func (nc *networkController) setupDockerNetwork(ctx context.Context) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("init Docker client: %w", err)
	}
	defer cli.Close()

	// Wait for the Docker daemon to start and be ready by sending a ping request in a loop.
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	ready, waitingLogged := false, false
	for !ready {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_, err = cli.Ping(ctx)
			if err == nil {
				ready = true
				break
			}
			if !client.IsErrConnectionFailed(err) {
				return fmt.Errorf("connect to Docker daemon: %w", err)
			}
			if !waitingLogged {
				slog.Info("Waiting for Docker daemon to start and be ready to setup Docker network.")
				waitingLogged = true
			}
		}
	}

	// Ensure the Docker network 'uncloud' is created with the correct subnet.
	needsCreation := false
	nw, err := cli.NetworkInspect(ctx, DockerNetworkName, dnetwork.InspectOptions{})
	if err != nil {
		if !client.IsErrNotFound(err) {
			return fmt.Errorf("inspect Docker network %q: %w", DockerNetworkName, err)
		}
		needsCreation = true
	} else if nw.IPAM.Config[0].Subnet != nc.state.Network.Subnet.String() {
		// Remove the Docker network if the subnet is different.
		// It could be a leftover from a previous incomplete cleanup.
		slog.Info(
			"Removing Docker network with old subnet.", "name", DockerNetworkName, "subnet", nw.IPAM.Config[0].Subnet,
		)
		if err = cli.NetworkRemove(ctx, DockerNetworkName); err != nil {
			// It can still fail if the network is in use by a container. Leave it to the user to resolve the issue.
			return fmt.Errorf("remove Docker network %q: %w", DockerNetworkName, err)
		}
		needsCreation = true
	}

	if needsCreation {
		if _, err = cli.NetworkCreate(
			ctx, DockerNetworkName, dnetwork.CreateOptions{
				Driver: "bridge",
				Scope:  "local",
				IPAM: &dnetwork.IPAM{
					Config: []dnetwork.IPAMConfig{
						{
							Subnet: nc.state.Network.Subnet.String(),
						},
					},
				},
			},
		); err != nil {
			return fmt.Errorf("create Docker network %q: %w", DockerNetworkName, err)
		}
		slog.Info("Docker network created.", "name", DockerNetworkName, "subnet", nc.state.Network.Subnet.String())

		if nw, err = cli.NetworkInspect(ctx, DockerNetworkName, dnetwork.InspectOptions{}); err != nil {
			return fmt.Errorf("inspect Docker network %q: %w", DockerNetworkName, err)
		}
	}

	// Configure iptables to allow WireGuard network to access containers. The Docker daemon should have already
	// created the DOCKER-USER chain at this point.
	// TODO: check if this works when firewalld used instead of raw iptables. The Docker daemon has a different
	//  code path for firewalld.

	// Bridge name doesn't seem to be documented but this is the source code where it is generated:
	// https://github.com/moby/moby/blob/v27.2.1/libnetwork/drivers/bridge/bridge_linux.go#L664
	bridgeName := "br-" + nw.ID[:12]
	ipt := iptables.GetIptable(iptables.IPv4)
	rule := []string{"--in-interface", network.WireGuardInterfaceName, "--out-interface", bridgeName, "-j", "ACCEPT"}
	if err = ipt.ProgramRule(iptables.Filter, DockerUserChain, iptables.Insert, rule); err != nil {
		return fmt.Errorf("insert iptables rule: %w", err)
	}

	return nil
}
