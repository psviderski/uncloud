package network

import (
	"fmt"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"net"
	"net/netip"
	"uncloud/internal/secret"
)

var (
	DefaultNetwork    = netip.MustParsePrefix("10.210.0.0/16")
	DefaultSubnetBits = 24
)

type Config struct {
	// Subnet is the IPv4 address range allocated to the machine. The machine's IP address is the first address
	// in the subnet. Other IP addresses are allocated to containers running on the machine.
	Subnet netip.Prefix
	// ManagementIP is the IPv6 address assigned to the machine within the WireGuard network. This address is used
	// for cluster management traffic, such as gRPC communication with the machine API server and Serf gossip.
	ManagementIP netip.Addr
	PrivateKey   secret.Secret
	PublicKey    secret.Secret
	Peers        []PeerConfig `json:",omitempty"`
}

type PeerConfig struct {
	Subnet *netip.Prefix `json:",omitempty"`
	// ManagementIP is the IPv6 address assigned to the peer within the WireGuard network. This address is used
	// for cluster management traffic, such as gRPC communication with the machine API server and Serf gossip.
	ManagementIP netip.Addr
	Endpoint     *netip.AddrPort  `json:",omitempty"`
	AllEndpoints []netip.AddrPort `json:",omitempty"`
	PublicKey    secret.Secret
}

// IsConfigured returns true if the configuration is complete to establish a WireGuard network.
func (c Config) IsConfigured() bool {
	return c.Subnet != (netip.Prefix{}) && c.ManagementIP != (netip.Addr{}) &&
		c.PrivateKey != nil && c.PublicKey != nil
}

func (c Config) toDeviceConfig() (wgtypes.Config, error) {
	privateKey, err := wgtypes.NewKey(c.PrivateKey)
	if err != nil {
		return wgtypes.Config{}, fmt.Errorf("parse private key: %w", err)
	}
	listenPort := WireGuardPort

	persistentKeepalive := WireGuardKeepaliveInterval
	wgPeerConfigs := make([]wgtypes.PeerConfig, len(c.Peers))
	for i, peerConfig := range c.Peers {
		peerPublicKey, kErr := wgtypes.NewKey(peerConfig.PublicKey)
		if kErr != nil {
			return wgtypes.Config{}, fmt.Errorf("parse peer public key: %w", kErr)
		}
		manageIP, mErr := addrToSingleIPPrefix(peerConfig.ManagementIP)
		if mErr != nil {
			return wgtypes.Config{}, fmt.Errorf("parse management IP: %w", mErr)
		}
		allowedIPs := []net.IPNet{prefixToIPNet(manageIP)}
		if peerConfig.Subnet != nil {
			allowedIPs = append(allowedIPs, prefixToIPNet(*peerConfig.Subnet))
		}
		wgPeerConfigs[i] = wgtypes.PeerConfig{
			PublicKey:                   peerPublicKey,
			ReplaceAllowedIPs:           true,
			AllowedIPs:                  allowedIPs,
			PersistentKeepaliveInterval: &persistentKeepalive,
		}
		if peerConfig.Endpoint != nil {
			wgPeerConfigs[i].Endpoint = &net.UDPAddr{
				IP:   peerConfig.Endpoint.Addr().AsSlice(),
				Port: int(peerConfig.Endpoint.Port()),
			}
		}
	}

	return wgtypes.Config{
		PrivateKey:   &privateKey,
		ListenPort:   &listenPort,
		ReplacePeers: true,
		Peers:        wgPeerConfigs,
	}, nil
}

func (p *PeerConfig) prefixes() ([]netip.Prefix, error) {
	managePrefix, err := addrToSingleIPPrefix(p.ManagementIP)
	if err != nil {
		return nil, fmt.Errorf("parse management IP: %w", err)
	}
	prefixes := []netip.Prefix{managePrefix}
	if p.Subnet != nil {
		prefixes = append(prefixes, *p.Subnet)
	}
	return prefixes, nil
}
