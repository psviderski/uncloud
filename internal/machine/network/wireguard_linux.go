//go:build linux

package network

import (
	"context"
	"errors"
	"fmt"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"golang.zx2c4.com/wireguard/wgctrl"
	"log/slog"
	"net/netip"
	"time"
)

type WireGuardNetwork struct {
	link  netlink.Link
	peers []peer
}

type peer struct {
	config                 PeerConfig
	lastEndpointChangeTime time.Time
}

func NewWireGuardNetwork() (*WireGuardNetwork, error) {
	link, err := createOrGetLink(WireGuardInterfaceName)
	if err != nil {
		return nil, fmt.Errorf("create or get WireGuard link %q: %v", WireGuardInterfaceName, err)
	}
	return &WireGuardNetwork{
		link: link,
	}, nil
}

// createOrGetLink creates a new WireGuard link with the given name if it doesn't already exist, otherwise it returns the existing link.
func createOrGetLink(name string) (netlink.Link, error) {
	link, err := netlink.LinkByName(name)
	if err == nil {
		slog.Info("Found existing WireGuard interface.", "name", name)
		return link, nil
	}
	//goland:noinspection GoTypeAssertionOnErrors
	if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return nil, fmt.Errorf("find WireGuard link %q: %v", name, err)
	}
	link = &netlink.GenericLink{
		// TODO: figure out how to set the most appropriate MTU.
		LinkAttrs: netlink.LinkAttrs{Name: name},
		LinkType:  "wireguard",
	}
	if err = netlink.LinkAdd(link); err != nil {
		return nil, fmt.Errorf("create WireGuard link %q: %v", name, err)
	}
	slog.Info("Created WireGuard interface.", "name", name)

	// Refetch the link to get the most up-to-date information.
	link, err = netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("find created WireGuard link %q: %v", name, err)
	}
	return link, nil
}

// Configure applies the given configuration to the WireGuard network interface.
// It updates device and peers settings, subnet, and peer routes.
func (n *WireGuardNetwork) Configure(config Config) error {
	// Reconstruct the list of peers, ensuring that the last endpoint change time is preserved for any existing peers.
	existingPeersByPublicKey := map[string]peer{}
	for _, p := range n.peers {
		existingPeersByPublicKey[p.config.PublicKey.String()] = p
	}
	n.peers = make([]peer, len(config.Peers))
	for i, peerConfig := range config.Peers {
		n.peers[i] = peer{
			config: peerConfig,
		}
		existingPeer, ok := existingPeersByPublicKey[peerConfig.PublicKey.String()]
		if ok && existingPeer.config.Endpoint == peerConfig.Endpoint {
			n.peers[i].lastEndpointChangeTime = existingPeer.lastEndpointChangeTime
		} else {
			n.peers[i].lastEndpointChangeTime = time.Now()
		}
	}

	wg, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("create WireGuard client: %w", err)
	}
	//goland:noinspection GoUnhandledErrorResult
	defer wg.Close()

	wgConfig, err := config.toDeviceConfig()
	if err != nil {
		return err
	}
	// Apply the new configuration to the WireGuard device.
	if err = wg.ConfigureDevice(n.link.Attrs().Name, wgConfig); err != nil {
		return fmt.Errorf("configure WireGuard device %q: %w", n.link.Attrs().Name, err)
	}
	slog.Info("Configured WireGuard interface.", "name", n.link.Attrs().Name)

	if err = n.updateSubnet(config.Subnet); err != nil {
		return err
	}
	slog.Info("Updated the subnet of the WireGuard interface.",
		"name", n.link.Attrs().Name, "subnet", config.Subnet)

	// Bring the WireGuard interface up if it's not already up.
	if n.link.Attrs().Flags&unix.IFF_UP != unix.IFF_UP {
		if err = netlink.LinkSetUp(n.link); err != nil {
			return fmt.Errorf("set WireGuard link %q up: %w", n.link.Attrs().Name, err)
		}
		slog.Info("Brought WireGuard interface up.", "name", n.link.Attrs().Name)
	}
	if err = n.updatePeerRoutes(); err != nil {
		return err
	}
	slog.Info("Updated routes to peers via the WireGuard interface.",
		"name", n.link.Attrs().Name, "peers", len(n.peers))

	return nil
}

// updateSubnet assigns the subnet and the first IP address in it to the WireGuard interface.
// It also removes any other addresses that have been added out of band.
func (n *WireGuardNetwork) updateSubnet(subnet netip.Prefix) error {
	machineIP := MachineIP(subnet)
	ipSubnet := prefixToIPNet(netip.PrefixFrom(machineIP, subnet.Bits()))
	if err := netlink.AddrAdd(n.link, &netlink.Addr{IPNet: &ipSubnet}); err != nil {
		if !errors.Is(err, unix.EEXIST) {
			return fmt.Errorf("add subnet address to WireGuard link %q: %w", n.link.Attrs().Name, err)
		}
	}
	// Remove the old subnet address if it has changed and remove any other addresses that have been added out of band.
	linkAddrs, err := netlink.AddrList(n.link, netlink.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("list addresses on WireGuard link %q: %w", n.link.Attrs().Name, err)
	}
	for _, addr := range linkAddrs {
		if addr.IPNet.String() == ipSubnet.String() {
			continue
		}
		if err = netlink.AddrDel(n.link, &addr); err != nil {
			return fmt.Errorf("remove address %q from WireGuard link %q: %w", addr.IPNet, n.link.Attrs().Name, err)
		}
	}
	return nil
}

// updatePeerRoutes adds routes to the peers via the WireGuard interface and removes old routes to peers
// that are no longer in the configuration.
func (n *WireGuardNetwork) updatePeerRoutes() error {
	// Add routes to the peers via the WireGuard link.
	for _, p := range n.peers {
		dst := prefixToIPNet(p.config.Subnet)
		if err := netlink.RouteAdd(&netlink.Route{
			LinkIndex: n.link.Attrs().Index,
			Scope:     netlink.SCOPE_LINK,
			Dst:       &dst,
		}); err != nil && !errors.Is(err, unix.EEXIST) {
			return fmt.Errorf("add route to WireGuard link %q: %w", n.link.Attrs().Name, err)
		}
		slog.Debug("Added route to peer via WireGuard interface.",
			"name", n.link.Attrs().Name, "peer", dst)
	}
	// Remove old routes to peers that are no longer in the configuration.
	routes, err := netlink.RouteList(n.link, netlink.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("list routes on WireGuard link %q: %w", n.link.Attrs().Name, err)
	}
	for _, route := range routes {
		old := true
		for _, p := range n.peers {
			if route.Dst.String() == p.config.Subnet.String() {
				old = false
				break
			}
		}
		if old {
			if err = netlink.RouteDel(&route); err != nil {
				return fmt.Errorf("remove route %q from WireGuard link %q: %w", route.Dst, n.link.Attrs().Name, err)
			}
			slog.Debug("Removed route to peer via WireGuard interface.",
				"name", n.link.Attrs().Name, "peer", route.Dst)
		}
	}
	return nil
}

func (n *WireGuardNetwork) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
