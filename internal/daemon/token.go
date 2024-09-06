package daemon

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"uncloud/internal/machine"
	"uncloud/internal/machine/network"
)

// MachineToken returns the local machine's token that can be used for adding the machine to a cluster.
// TODO: ideally, this should be an RPC call to the daemon API to ensure the config is created and up-to-date.
func MachineToken(dataDir string) (machine.Token, error) {
	state, err := machine.ParseState(machine.StatePath(dataDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return machine.Token{}, fmt.Errorf("load machine config (is uncloudd daemon running?): %w", err)
		}
		return machine.Token{}, fmt.Errorf("load machine config: %w", err)
	}
	if len(state.Network.PublicKey) == 0 {
		return machine.Token{}, errors.New("public key is not set in machine config")
	}

	ips, err := network.ListRoutableIPs()
	if err != nil {
		return machine.Token{}, fmt.Errorf("list routable addresses: %w", err)
	}
	publicIP, err := network.GetPublicIP()
	// Ignore the error if failed to get the public IP using API services.
	if err == nil {
		ips = append(ips, publicIP)
	}

	endpoints := make([]netip.AddrPort, len(ips))
	for i, ip := range ips {
		endpoints[i] = netip.AddrPortFrom(ip, network.WireGuardPort)
	}
	return machine.NewToken(state.Network.PublicKey, endpoints), nil
}
