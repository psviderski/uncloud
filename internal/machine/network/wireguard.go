package network

import (
	"fmt"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"net/netip"
	"time"
	"uncloud/internal/secret"
)

const (
	WireGuardInterfaceName = "uncloud"
	WireGuardPort          = 51820
	// WireGuardKeepaliveInterval is sensible interval that works with a wide variety of firewalls.
	WireGuardKeepaliveInterval = 25 * time.Second
)

type EndpointChangeEvent struct {
	PublicKey secret.Secret
	// Endpoint is the new endpoint of the peer.
	Endpoint netip.AddrPort
}

// NewMachineKeys generates a new WireGuard private and public key pair.
func NewMachineKeys() (privKey, pubKey secret.Secret, err error) {
	wgPrivKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return nil, nil, fmt.Errorf("generate WireGuard private key: %w", err)
	}
	privKey = wgPrivKey[:]
	wgPubKey := wgPrivKey.PublicKey()
	pubKey = wgPubKey[:]
	return
}
