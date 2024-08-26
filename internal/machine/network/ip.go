package network

import (
	"net"
	"net/netip"
)

// MachineIP returns the IP address of the machine which is the first address in the subnet.
func MachineIP(subnet netip.Prefix) netip.Addr {
	return subnet.Masked().Addr().Next()
}

func prefixToIPNet(prefix netip.Prefix) net.IPNet {
	return net.IPNet{
		IP:   prefix.Addr().AsSlice(),
		Mask: net.CIDRMask(prefix.Bits(), prefix.Addr().BitLen()),
	}
}
