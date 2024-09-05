package pb

import (
	"bytes"
	"fmt"
	"net/netip"
)

func NewIP(addr netip.Addr) *IP {
	// MarshalBinary always returns a nil error.
	ip, _ := addr.MarshalBinary()
	return &IP{Ip: ip}
}

func (ip *IP) ToAddr() (netip.Addr, error) {
	var addr netip.Addr
	if err := addr.UnmarshalBinary(ip.Ip); err != nil {
		return netip.Addr{}, fmt.Errorf("unmarshal IP: %w", err)
	}
	return addr, nil
}

func (ip *IP) Equal(other *IP) bool {
	return bytes.Equal(ip.Ip, other.Ip)
}

func NewIPPort(ap netip.AddrPort) *IPPort {
	return &IPPort{Ip: NewIP(ap.Addr()), Port: uint32(ap.Port())}
}

func (ipp *IPPort) ToAddrPort() (netip.AddrPort, error) {
	addr, err := ipp.Ip.ToAddr()
	if err != nil {
		return netip.AddrPort{}, err
	}
	return netip.AddrPortFrom(addr, uint16(ipp.Port)), nil
}

func NewIPPrefix(p netip.Prefix) *IPPrefix {
	return &IPPrefix{Ip: NewIP(p.Addr()), Bits: uint32(p.Bits())}
}

func (p *IPPrefix) ToPrefix() (netip.Prefix, error) {
	addr, err := p.Ip.ToAddr()
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(addr, int(p.Bits)), nil
}
