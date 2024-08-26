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
	Subnet     netip.Prefix
	PrivateKey secret.Secret
	PublicKey  secret.Secret
	Peers      []PeerConfig
}

type PeerConfig struct {
	Subnet       netip.Prefix
	Endpoint     netip.AddrPort
	AllEndpoints []netip.AddrPort
	PublicKey    secret.Secret
}

func (c Config) toDeviceConfig() (wgtypes.Config, error) {
	privateKey, err := wgtypes.NewKey(c.PrivateKey)
	if err != nil {
		panic(fmt.Errorf("parse private key: %w", err))
	}
	listenPort := WireGuardPort

	persistentKeepalive := WireGuardKeepaliveInterval
	wgPeerConfigs := make([]wgtypes.PeerConfig, len(c.Peers))
	for i, peerConfig := range c.Peers {
		peerPublicKey, kErr := wgtypes.NewKey(peerConfig.PublicKey)
		if kErr != nil {
			return wgtypes.Config{}, fmt.Errorf("parse peer public key: %w", kErr)
		}
		endpoint := &net.UDPAddr{
			IP:   peerConfig.Endpoint.Addr().AsSlice(),
			Port: int(peerConfig.Endpoint.Port()),
		}
		wgPeerConfigs[i] = wgtypes.PeerConfig{
			PublicKey:                   peerPublicKey,
			Endpoint:                    endpoint,
			ReplaceAllowedIPs:           true,
			AllowedIPs:                  []net.IPNet{prefixToIPNet(peerConfig.Subnet)},
			PersistentKeepaliveInterval: &persistentKeepalive,
		}
	}

	return wgtypes.Config{
		PrivateKey:   &privateKey,
		ListenPort:   &listenPort,
		ReplacePeers: true,
		Peers:        wgPeerConfigs,
	}, nil
}
