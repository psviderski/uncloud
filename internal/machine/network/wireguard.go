package network

import (
	"fmt"
	"net/netip"
	"time"

	"github.com/psviderski/uncloud/internal/secret"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const (
	WireGuardInterfaceName = "uncloud"
	DefaultWireGuardPort   = 51820
	// MinWireGuardMTU is the minimum MTU for the WireGuard interface. The management traffic inside the tunnel uses
	// IPv6 whose minimum link MTU is 1280, so this is a safe floor that also keeps Corrosion's max_mtu (>= 1200) valid.
	MinWireGuardMTU = 1280
	// MaxWireGuardMTU is the conservative maximum MTU set by auto-detection and the fallback when detection fails.
	// It's the standard WireGuard MTU for a 1500-byte underlay (1500 - 80) that matches the kernel's default
	// for WireGuard links.
	MaxWireGuardMTU = 1500 - wireGuardEncapOverhead
	// wireGuardEncapOverhead is WireGuard's worst-case (IPv6 endpoint) encapsulation overhead: outer IPv6 (40) +
	// UDP (8) + WireGuard message header and auth tag (32).
	wireGuardEncapOverhead = 80
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
