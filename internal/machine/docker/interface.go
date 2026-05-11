package docker

import (
	"fmt"
	"net"
	"net/netip"
)

// addrOfPrefix checks the interface and returns the address of each interface that container the prefix.
func addrOfPrefix(prefix netip.Prefix) ([]string, error) {
	ifis, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var addrs []string
	for _, ifi := range ifis {
		ifaddrs, _ := ifi.Addrs()
		println(len(ifaddrs))
		for _, addr := range ifaddrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			nip, _ := netip.ParseAddr(ipnet.IP.String()) // round about way is needed to get ipv6 addrs, not mapped v4 in v6.
			if prefix.Contains(nip) {
				addrs = append(addrs, nip.String())
			}
		}
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses are contained in prefix '%s'", prefix)
	}

	return addrs, nil
}
