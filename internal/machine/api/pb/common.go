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
	if !addr.IsValid() {
		return netip.Addr{}, fmt.Errorf("invalid IP")
	}
	return addr, nil
}

func (ip *IP) ToString() string {
	// Has to be ToString, because String is generated (and doesn't do the right thing).
	if ip == nil {
		return ""
	}
	addr, _ := ip.ToAddr()
	return addr.String()
}

func (ip *IP) Equal(other *IP) bool {
	return bytes.Equal(ip.Ip, other.Ip)
}

func (ip *IP) MarshalJSON() ([]byte, error) {
	return []byte("\"" + ip.ToString() + "\""), nil
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

func (ipp *IPPort) ToString() string {
	// Has to be ToString, because String is generated (and doesn't do the right thing).
	if ipp == nil {
		return ""
	}
	addrPort, _ := ipp.ToAddrPort()
	return addrPort.String()
}

func (ipp *IPPort) MarshalJSON() ([]byte, error) {
	return []byte("\"" + ipp.ToString() + "\""), nil
}

func NewIPPrefix(p netip.Prefix) *IPPrefix {
	return &IPPrefix{Ip: NewIP(p.Addr()), Bits: uint32(p.Bits())}
}

func (p *IPPrefix) ToPrefix() (netip.Prefix, error) {
	addr, err := p.Ip.ToAddr()
	if err != nil {
		return netip.Prefix{}, err
	}
	prefix := netip.PrefixFrom(addr, int(p.Bits))
	if !prefix.IsValid() {
		return netip.Prefix{}, fmt.Errorf("invalid prefix")
	}
	return prefix, nil
}
