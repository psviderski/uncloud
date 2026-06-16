//go:build linux

package network

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

// detectEgressMTU returns the MTU of the egress network interface.
// It resolves the route to a public address to find the egress interface.
func detectEgressMTU() (int, error) {
	// Resolve the route to a public address to determine the egress interface.
	routes, err := netlink.RouteGet(net.IPv4(1, 1, 1, 1))
	if err != nil {
		return 0, fmt.Errorf("get route to public address: %w", err)
	}
	if len(routes) == 0 {
		return 0, fmt.Errorf("no route to public address")
	}
	route := routes[0]

	link, err := netlink.LinkByIndex(route.LinkIndex)
	if err != nil {
		return 0, fmt.Errorf("get egress interface for route: %w", err)
	}
	// Don't detect the MTU from the WireGuard interface itself if the default route happens to go through it.
	if link.Attrs().Name == WireGuardInterfaceName {
		return 0, fmt.Errorf("egress interface is the WireGuard interface '%s'", WireGuardInterfaceName)
	}

	// Prefer the route-level MTU (e.g. set by PMTU discovery) over the interface MTU if present.
	if route.MTU > 0 {
		return route.MTU, nil
	}
	return link.Attrs().MTU, nil
}
