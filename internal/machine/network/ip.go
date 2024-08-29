package network

import (
	"net"
	"net/netip"
	"uncloud/internal/secret"
)

// MachineIP returns the IP address of the machine which is the first address in the subnet.
func MachineIP(subnet netip.Prefix) netip.Addr {
	return subnet.Masked().Addr().Next()
}

// PeerIPv6 returns the IPv6 address of a peer derived from the first 14 bytes of its public key.
func PeerIPv6(publicKey secret.Secret) netip.Addr {
	bytes := [16]byte{0xfd, 0xcc}
	copy(bytes[2:], publicKey[:14])
	return netip.AddrFrom16(bytes)
}

func prefixToIPNet(prefix netip.Prefix) net.IPNet {
	return net.IPNet{
		IP:   prefix.Addr().AsSlice(),
		Mask: net.CIDRMask(prefix.Bits(), prefix.Addr().BitLen()),
	}
}
