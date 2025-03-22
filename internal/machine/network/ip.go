package network

import (
	"fmt"
	"github.com/psviderski/uncloud/internal/secret"
	"net"
	"net/netip"
)

// MachineIP returns the IP address of the machine which is the first address in the subnet.
func MachineIP(subnet netip.Prefix) netip.Addr {
	return subnet.Masked().Addr().Next()
}

// ManagementIP returns the IPv6 address of a peer derived from the first 14 bytes of its public key.
// This address is intended for cluster management traffic.
func ManagementIP(publicKey secret.Secret) netip.Addr {
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

// ipNetToPrefix returns a netip.Prefix from the net.IPNet type. If ipNet is invalid, ok is false.
// Based on https://github.com/tailscale/tailscale/blob/main/net/netaddr/netaddr.go
func ipNetToPrefix(ipNet net.IPNet) (netip.Prefix, error) {
	ip, ok := netip.AddrFromSlice(ipNet.IP)
	if !ok {
		return netip.Prefix{}, fmt.Errorf("invalid IP network")
	}
	ip = ip.Unmap()

	if l := len(ipNet.Mask); l != net.IPv4len && l != net.IPv6len {
		return netip.Prefix{}, fmt.Errorf("invalid IP network mask length: %d", l)
	}

	ones, bits := ipNet.Mask.Size()
	if ones == 0 && bits == 0 {
		return netip.Prefix{}, fmt.Errorf("non-contiguous IP network mask")
	}

	return netip.PrefixFrom(ip, ones), nil
}

func addrToSingleIPPrefix(addr netip.Addr) (netip.Prefix, error) {
	if !addr.IsValid() {
		return netip.Prefix{}, fmt.Errorf("invalid IP address")
	}
	bits := 32
	if addr.Is6() {
		bits = 128
	}
	return addr.Prefix(bits)
}
