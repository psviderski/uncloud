package docker

import (
	"context"
	"fmt"
	dnetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/libnetwork/iptables"
	"log/slog"
	"net/netip"
	"uncloud/internal/machine/network"
)

// EnsureUncloudNetwork creates the Docker bridge network NetworkName with the provided machine subnet
// if it doesn't exist. If the network exists but has a different subnet, it removes and recreates the network.
// It also configures iptables to allow container access from the WireGuard network.
func (m *Manager) EnsureUncloudNetwork(ctx context.Context, subnet netip.Prefix) error {
	// Ensure the Docker network 'uncloud' is created with the correct subnet.
	needsCreation := false
	nw, err := m.client.NetworkInspect(ctx, NetworkName, dnetwork.InspectOptions{})
	if err != nil {
		if !client.IsErrNotFound(err) {
			return fmt.Errorf("inspect Docker network %q: %w", NetworkName, err)
		}
		needsCreation = true
	} else if nw.IPAM.Config[0].Subnet != subnet.String() {
		// Remove the Docker network if the subnet is different.
		// It could be a leftover from a previous incomplete cleanup.
		slog.Info(
			"Removing Docker network with old subnet.", "name", NetworkName, "subnet", nw.IPAM.Config[0].Subnet,
		)
		if err = m.client.NetworkRemove(ctx, NetworkName); err != nil {
			// It can still fail if the network is in use by a container. Leave it to the user to resolve the issue.
			return fmt.Errorf("remove Docker network %q: %w", NetworkName, err)
		}
		needsCreation = true
	}

	if needsCreation {
		if _, err = m.client.NetworkCreate(
			ctx, NetworkName, dnetwork.CreateOptions{
				Driver: "bridge",
				Scope:  "local",
				IPAM: &dnetwork.IPAM{
					Config: []dnetwork.IPAMConfig{
						{
							Subnet: subnet.String(),
						},
					},
				},
			},
		); err != nil {
			return fmt.Errorf("create Docker network %q: %w", NetworkName, err)
		}
		slog.Info("Docker network created.", "name", NetworkName, "subnet", subnet.String())

		if nw, err = m.client.NetworkInspect(ctx, NetworkName, dnetwork.InspectOptions{}); err != nil {
			return fmt.Errorf("inspect Docker network %q: %w", NetworkName, err)
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
	if err = ipt.ProgramRule(iptables.Filter, UserChain, iptables.Insert, rule); err != nil {
		return fmt.Errorf("insert iptables rule: %w", err)
	}

	return nil
}
