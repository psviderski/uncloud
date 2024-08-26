package daemon

import (
	"context"
	"fmt"
	"uncloud/internal/machine"
	"uncloud/internal/machine/network"
)

func Run(ctx context.Context, dataDir string) error {
	cfg, err := machine.ParseConfig(machine.ConfigPath(dataDir))
	if err != nil {
		return fmt.Errorf("load machine config: %w", err)
	}

	wgnet, err := network.NewWireGuardNetwork()
	if err != nil {
		return fmt.Errorf("create WireGuard network: %w", err)
	}
	if err = wgnet.Configure(*cfg.Network); err != nil {
		return fmt.Errorf("configure WireGuard network: %w", err)
	}

	//ctx, cancel := context.WithCancel(context.Background())
	//go wgnet.WatchEndpoints(ctx, peerEndpointChangeNotifier)

	//addrs, err := network.ListRoutableAddresses()
	//if err != nil {
	//	return err
	//}
	//fmt.Println("Addresses:", addrs)

	return wgnet.Run(ctx)
}
