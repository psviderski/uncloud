package network

import "log/slog"

// DetectMTU returns the optimal MTU for the WireGuard interface based on the machine's egress network.
// The egress MTU is capped at MaxWireGuardMTU to not overestimate the path MTU between machines which can go over
// the public internet. If the egress MTU cannot be detected, it falls back to MaxWireGuardMTU.
func DetectMTU() int {
	egressMTU, err := detectEgressMTU()
	if err != nil {
		slog.Warn("Failed to detect egress network MTU, falling back to the default WireGuard MTU.",
			"mtu", MaxWireGuardMTU, "err", err)
		return MaxWireGuardMTU
	}

	mtu := egressMTU - wireGuardEncapOverhead
	// Clamp the computed MTU to the range [MinWireGuardMTU, MaxWireGuardMTU].
	mtu = min(max(mtu, MinWireGuardMTU), MaxWireGuardMTU)
	slog.Info("Detected optimal WireGuard MTU from the egress network.", "mtu", mtu, "egress_mtu", egressMTU)

	return mtu
}
