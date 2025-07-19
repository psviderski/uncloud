package cluster

import (
	"errors"
	"fmt"
	"net/netip"

	"go4.org/netipx"
)

const DefaultSubnetBits = 24

var DefaultNetwork = netip.MustParsePrefix("10.210.0.0/16")

// IPAM is an in-memory IP address manager for allocating and releasing subnets for machines from a cluster network.
type IPAM struct {
	network   netip.Prefix
	allocated netipx.IPSetBuilder
}

func NewIPAM(network netip.Prefix) (*IPAM, error) {
	if !network.IsValid() || network.Bits() == 0 {
		return nil, errors.New("invalid network")
	}
	return &IPAM{
		network: network.Masked(),
	}, nil
}

// NewIPAMWithAllocated creates a new IPAM with the given network and already allocated subnets.
func NewIPAMWithAllocated(network netip.Prefix, subnets []netip.Prefix) (*IPAM, error) {
	ipam, err := NewIPAM(network)
	if err != nil {
		return nil, err
	}
	for _, sn := range subnets {
		if err = ipam.AllocateSubnet(sn); err != nil {
			return nil, fmt.Errorf("allocate subnet %s: %w", sn, err)
		}
	}
	return ipam, nil
}

func (ipam *IPAM) AllocateSubnetLen(bits int) (netip.Prefix, error) {
	if bits < ipam.network.Bits() || bits > ipam.network.Addr().BitLen() {
		return netip.Prefix{}, errors.New("invalid subnet size")
	}

	subnet := netip.PrefixFrom(ipam.network.Addr(), bits)
	for ipam.network.Contains(subnet.Addr()) {
		ipset, err := ipam.allocated.IPSet()
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("get allocated IP set: %w", err)
		}
		if !ipset.OverlapsPrefix(subnet) {
			ipam.allocated.AddPrefix(subnet)
			return subnet, nil
		}

		nextSubnetAddr := netipx.PrefixLastIP(subnet).Next()
		subnet = netip.PrefixFrom(nextSubnetAddr, bits)
	}

	return netip.Prefix{}, errors.New("no available subnet")
}

func (ipam *IPAM) AllocateSubnet(subnet netip.Prefix) error {
	subnetLastIP := netipx.PrefixLastIP(subnet)
	if !ipam.network.Contains(subnet.Addr()) || !ipam.network.Contains(subnetLastIP) {
		return errors.New("subnet not in network")
	}
	ipset, err := ipam.allocated.IPSet()
	if err != nil {
		return fmt.Errorf("get allocated IP set: %w", err)
	}
	if ipset.OverlapsPrefix(subnet) {
		return errors.New("subnet overlaps with allocated subnets")
	}
	ipam.allocated.AddPrefix(subnet)
	return nil
}
