package docker

import (
	"fmt"
	"net"
	"net/netip"
)

// cgnatPrefix is the shared address space for carrier-grade NAT. Addresses in it are not private in the
// RFC 1918 sense but are not globally routable either.
var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// interfaceAddrs returns the IP address of every interface on the host.
func interfaceAddrs() ([]netip.Addr, error) {
	ifis, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var addrs []netip.Addr
	for _, ifi := range ifis {
		ifaddrs, _ := ifi.Addrs()
		for _, addr := range ifaddrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			// The round about way is needed to get ipv6 addrs, not mapped v4 in v6.
			if nip, err := netip.ParseAddr(ipnet.IP.String()); err == nil {
				addrs = append(addrs, nip)
			}
		}
	}

	return addrs, nil
}

// addrOfPrefix checks the interfaces and returns the address of each interface that was contained in the prefix.
func addrOfPrefix(prefix netip.Prefix) ([]string, error) {
	ifaceAddrs, err := interfaceAddrs()
	if err != nil {
		return nil, err
	}

	var addrs []string
	for _, addr := range ifaceAddrs {
		if prefix.Contains(addr) {
			addrs = append(addrs, addr.String())
		}
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no host addresses are contained in prefix '%s'", prefix)
	}

	return addrs, nil
}

// publicAddrs returns the globally routable IP address of each interface on the host.
func publicAddrs() ([]string, error) {
	ifaceAddrs, err := interfaceAddrs()
	if err != nil {
		return nil, err
	}

	addrs := filterPublicAddrs(ifaceAddrs)
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no public IP addresses found on the host")
	}

	return addrs, nil
}

// filterPublicAddrs returns the globally routable addresses among addrs.
func filterPublicAddrs(addrs []netip.Addr) []string {
	var public []string
	for _, addr := range addrs {
		if isPublicAddr(addr) {
			public = append(public, addr.String())
		}
	}

	return public
}

// isPublicAddr reports whether addr is globally routable.
func isPublicAddr(addr netip.Addr) bool {
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsLoopback() || addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() {
		return false
	}

	return !(addr.Is4() && cgnatPrefix.Contains(addr))
}
