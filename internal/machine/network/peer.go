package network

import (
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"log/slog"
	"time"
)

const (
	PeerStatusUnknown = "unknown"
	PeerStatusUp      = "up"
	PeerStatusDown    = "down"
)

type peer struct {
	config                 PeerConfig
	lastEndpointChangeTime time.Time
	lastHandshakeTime      time.Time
	receiveBytes           int64
	transmitBytes          int64
	status                 string
}

func newPeer(config PeerConfig) *peer {
	p := &peer{
		config: config,
		status: PeerStatusUnknown,
	}
	if p.config.Endpoint != nil {
		p.lastEndpointChangeTime = time.Now()
	}
	return p
}

func (p *peer) updateConfig(config PeerConfig) {
	if p.config.Endpoint != config.Endpoint {
		p.lastEndpointChangeTime = time.Now()
		p.status = PeerStatusUnknown
	}
	p.config = config
}

func (p *peer) updateFromWireGuard(wgPeer wgtypes.Peer) {
	p.lastHandshakeTime = wgPeer.LastHandshakeTime
	p.receiveBytes = wgPeer.ReceiveBytes
	p.transmitBytes = wgPeer.TransmitBytes
	p.calculateStatus()
}

// Peer status calculation is based on Talos Kubespan implementation:
// https://github.com/siderolabs/talos/blob/v1.8.0/internal/app/machined/pkg/adapters/kubespan/peer_status.go

// endpointConnectionTimeout is time to wait for initial handshake when the endpoint is just set.
const endpointConnectionTimeout = 15 * time.Second

// peerDownInterval is the time since last handshake when established peer is considered to be down.
//
// WG whitepaper defines a downed peer as being:
// Handshake Timeout (180s) + Rekey Timeout (5s) + Rekey Attempt Timeout (90s)
//
// This interval is applied when the link is already established.
const peerDownInterval = (180 + 5 + 90) * time.Second

// calculateStatus updates the peer's connection status based on other field values.
//
// Goal: endpoint is ultimately down if we haven't seen handshake for more than peerDownInterval,
// but as the endpoints get updated we want faster feedback, so we start checking more aggressively
// that the handshake happened within endpointConnectionTimeout since last endpoint change.
//
// Timeline:
//
// ---------------------------------------------------------------------->
// ^            ^                                   ^
// |            |                                   |
// T0           T0+endpointConnectionTimeout        T0+peerDownInterval
//
// Where T0 = lastEndpointChangeTime
//
// The question is where is LastHandshakeTimeout vs. those points above:
//
//   - if we're past (T0+peerDownInterval), simply check that time since last handshake < peerDownInterval
//   - if we're between (T0+endpointConnectionTimeout) and (T0+peerDownInterval), and there's no handshake
//     after the endpoint change, assume that the endpoint is down
//   - if we're between (T0) and (T0+endpointConnectionTimeout), and there's no handshake since the endpoint change,
//     consider the state to be unknown
func (p *peer) calculateStatus() {
	lastStatus := p.status
	sinceLastHandshake := time.Since(p.lastHandshakeTime)
	sinceEndpointChange := time.Since(p.lastEndpointChangeTime)

	switch {
	case sinceEndpointChange > peerDownInterval: // past T0+peerDownInterval
		// If we got handshake in the last peerDownInterval, endpoint is up.
		if sinceLastHandshake < peerDownInterval {
			p.status = PeerStatusUp
		} else {
			p.status = PeerStatusDown
		}
	case sinceEndpointChange < endpointConnectionTimeout: // between (T0) and (T0+endpointConnectionTimeout)
		// Endpoint got recently updated, consider no handshake as 'unknown'.
		if p.lastHandshakeTime.After(p.lastEndpointChangeTime) {
			p.status = PeerStatusUp
		} else {
			p.status = PeerStatusUnknown
		}
	default: // otherwise, we're between (T0+endpointConnectionTimeout) and (T0+peerDownInterval)
		// If we haven't had the handshake yet, consider the endpoint to be down.
		if p.lastHandshakeTime.After(p.lastEndpointChangeTime) {
			p.status = PeerStatusUp
		} else {
			p.status = PeerStatusDown
		}
	}

	if p.status == PeerStatusDown && p.config.Endpoint == nil {
		// No endpoint, so unknown.
		p.status = PeerStatusUnknown
	}
	if p.status != lastStatus {
		slog.Debug("Peer status changed.", "public_key", p.config.PublicKey, "status", p.status)
	}
}
