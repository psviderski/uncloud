package docker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strconv"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dnetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/libnetwork/iptables"
	"github.com/psviderski/uncloud/internal/machine/dns"
	"github.com/psviderski/uncloud/internal/machine/firewall"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/psviderski/uncloud/pkg/api"
)

// EnsureUncloudNetwork creates the Docker bridge network NetworkName with the provided machine subnet
// if it doesn't exist. If the network exists but has a different subnet, it removes and recreates the network.
// It also configures iptables to allow container access from the WireGuard network.
func (c *Controller) EnsureUncloudNetwork(ctx context.Context, subnet netip.Prefix, dnsServer netip.Addr) error {
	// Ensure the Docker network 'uncloud' is created with the correct subnet.
	needsCreation := false
	nw, err := c.client.NetworkInspect(ctx, NetworkName, dnetwork.InspectOptions{})
	if err != nil {
		if !client.IsErrNotFound(err) {
			return fmt.Errorf("inspect Docker network '%s': %w", NetworkName, err)
		}
		needsCreation = true
	} else if nw.IPAM.Config[0].Subnet != subnet.String() {
		// Remove the Docker network if the subnet is different.
		// It could be a leftover from a previous incomplete cleanup.
		slog.Info(
			"Removing Docker network with old subnet.", "name", NetworkName, "subnet", nw.IPAM.Config[0].Subnet,
		)
		if err = c.client.NetworkRemove(ctx, NetworkName); err != nil {
			// It can still fail if the network is in use by a container. Leave it to the user to resolve the issue.
			return fmt.Errorf("remove Docker network '%s': %w", NetworkName, err)
		}
		needsCreation = true
	}

	if needsCreation {
		if _, err = c.client.NetworkCreate(
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
				Labels: map[string]string{
					api.LabelManaged: "",
				},
				Options: map[string]string{
					// Starting with Docker 28.2.0 (https://github.com/moby/moby/pull/49832), we have to explicitly
					// allow direct routing from the WireGuard interface to the bridge network.
					"com.docker.network.bridge.trusted_host_interfaces": network.WireGuardInterfaceName,
				},
			},
		); err != nil {
			return fmt.Errorf("create Docker network '%s': %w", NetworkName, err)
		}
		slog.Info("Docker network created.", "name", NetworkName, "subnet", subnet.String())

		if nw, err = c.client.NetworkInspect(ctx, NetworkName, dnetwork.InspectOptions{}); err != nil {
			return fmt.Errorf("inspect Docker network '%s': %w", NetworkName, err)
		}
	}

	// Configure iptables to allow WireGuard network to access containers. The Docker daemon should have already
	// created the DOCKER-USER chain at this point.
	// TODO: check if this works when firewalld used instead of raw iptables. The Docker daemon has a different
	//  code path for firewalld.

	// Bridge name doesn't seem to be documented but this is the source code where it is generated:
	// https://github.com/moby/moby/blob/v27.2.1/libnetwork/drivers/bridge/bridge_linux.go#L664
	bridgeName := "br-" + nw.ID[:12]

	if err = configureIptables(bridgeName, subnet, dnsServer); err != nil {
		return fmt.Errorf("configure iptables for Docker network '%s': %w", NetworkName, err)
	}

	return nil
}

// configureIptables configures iptables rules for the uncloud Docker network.
func configureIptables(bridgeName string, subnet netip.Prefix, dnsServer netip.Addr) error {
	ipt := iptables.GetIptable(iptables.IPv4)
	// Allow traffic from other machines and their containers through the WG mesh to the Uncloud containers
	// on the machine.
	wgRule := []string{
		"--in-interface", network.WireGuardInterfaceName,
		"--out-interface", bridgeName,
		"-j", "ACCEPT",
	}
	if err := ipt.ProgramRule(iptables.Filter, firewall.DockerUserChain, iptables.Insert, wgRule); err != nil {
		return fmt.Errorf("insert iptables rule: %w", err)
	}

	// Allow DNS queries from Uncloud containers to the embedded DNS server.
	for _, proto := range []string{"udp", "tcp"} {
		dnsRule := []string{
			"--in-interface", bridgeName,
			"--dst", dnsServer.String(),
			"--protocol", proto,
			"--dport", strconv.Itoa(dns.Port),
			"-j", "ACCEPT",
		}
		if err := ipt.ProgramRule(iptables.Filter, firewall.UncloudInputChain, iptables.Insert, dnsRule); err != nil {
			return fmt.Errorf("insert iptables rule: %w", err)
		}
	}

	// Skip masquerading for the container traffic going from the uncloud Docker network through the WG mesh.
	// https://uncloud.run/blog/connect-docker-containers-across-hosts-wireguard#step-3-configure-ip-routing
	skipMasqueradeRule := []string{
		"--src", subnet.String(),
		"--out-interface", network.WireGuardInterfaceName,
		"-j", "RETURN",
	}
	// Delete and reinsert the rule to ensure it's at the top of the POSTROUTING chain before the MASQUERADE rule
	// added by Docker: POSTROUTING -s 10.210.X.0/24 ! -o br-XXX -j MASQUERADE
	if err := ipt.ProgramRule(iptables.Nat, "POSTROUTING", iptables.Delete, skipMasqueradeRule); err != nil {
		return fmt.Errorf("delete iptables rule: %w", err)
	}
	if err := ipt.ProgramRule(iptables.Nat, "POSTROUTING", iptables.Insert, skipMasqueradeRule); err != nil {
		return fmt.Errorf("insert iptables rule: %w", err)
	}

	return nil
}

// cleanupIptables deletes the iptables rules for the uncloud Docker network.
func cleanupIptables(bridgeName string, subnet netip.Prefix) error {
	ipt := iptables.GetIptable(iptables.IPv4)
	// Delete the rule allowing traffic from the WireGuard network to the Docker bridge.
	wgRule := []string{
		"--in-interface", network.WireGuardInterfaceName,
		"--out-interface", bridgeName,
		"-j", "ACCEPT",
	}
	if err := ipt.ProgramRule(iptables.Filter, firewall.DockerUserChain, iptables.Delete, wgRule); err != nil {
		return fmt.Errorf("delete iptables rule: %w", err)
	}

	// Delete the rule that skips masquerading for the container traffic going from the uncloud Docker network
	// through the WG mesh.
	skipMasqueradeRule := []string{
		"--src", subnet.String(),
		"--out-interface", network.WireGuardInterfaceName,
		"-j", "RETURN",
	}
	if err := ipt.ProgramRule(iptables.Nat, "POSTROUTING", iptables.Delete, skipMasqueradeRule); err != nil {
		return fmt.Errorf("delete iptables rule: %w", err)
	}

	// Rules in uncloud-owned chains will be automatically cleaned up by the machine cleanup.

	return nil
}

// Cleanup removes all uncloud-managed containers and the uncloud Docker network.
func (c *Controller) Cleanup() error {
	ctx := context.Background()
	var errs []error

	// Remove uncloud-managed Docker containers.
	containers, err := c.client.ContainerList(ctx, dockercontainer.ListOptions{
		All: true, // Include stopped containers.
		Filters: filters.NewArgs(
			filters.Arg("label", api.LabelManaged),
		),
	})
	if err != nil {
		errs = append(errs, fmt.Errorf("list uncloud-managed Docker containers: %w", err))
	} else if len(containers) > 0 {
		slog.Info("Removing uncloud-managed Docker containers.", "count", len(containers))
		removed := 0

		for _, ctr := range containers {
			err = c.client.ContainerStop(ctx, ctr.ID, dockercontainer.StopOptions{})
			if err != nil && !client.IsErrNotFound(err) {
				errs = append(errs, fmt.Errorf("stop container '%s': %w", ctr.ID, err))
			}

			err = c.client.ContainerRemove(ctx, ctr.ID, dockercontainer.RemoveOptions{
				// Remove anonymous volumes created by the container.
				RemoveVolumes: true,
			})
			if err == nil {
				removed++
			} else if !client.IsErrNotFound(err) {
				errs = append(errs, fmt.Errorf("remove container '%s': %w", ctr.ID, err))
			}
		}
		slog.Info("Removed uncloud-managed Docker containers.", "count", removed)
	}

	// Remove the uncloud Docker network and related iptables rules.
	nw, err := c.client.NetworkInspect(ctx, NetworkName, dnetwork.InspectOptions{})
	if err == nil {
		bridgeName := "br-" + nw.ID[:12]
		var subnet netip.Prefix
		if len(nw.IPAM.Config) > 0 {
			subnet, _ = netip.ParsePrefix(nw.IPAM.Config[0].Subnet)
		}

		if subnet.IsValid() {
			if err = cleanupIptables(bridgeName, subnet); err != nil {
				errs = append(errs, fmt.Errorf("cleanup iptables for Docker network '%s': %w", NetworkName, err))
			} else {
				slog.Info("Cleaned up iptables rules for Docker network.", "name", NetworkName, "bridge", bridgeName)
			}
		}

		if err = c.client.NetworkRemove(ctx, NetworkName); err == nil {
			slog.Info("Docker network removed.", "name", NetworkName)
		} else if !client.IsErrNotFound(err) {
			errs = append(errs, fmt.Errorf("remove Docker network '%s': %w", NetworkName, err))
		}
	} else if !client.IsErrNotFound(err) {
		errs = append(errs, fmt.Errorf("inspect Docker network '%s': %w", NetworkName, err))
	}

	return errors.Join(errs...)
}
