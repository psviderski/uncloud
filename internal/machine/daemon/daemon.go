package daemon

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sync/errgroup"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strconv"
	"uncloud/internal/machine"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/cluster"
	"uncloud/internal/machine/network"
)

// InitCluster resets the local machine and initialises a new cluster with it.
// TODO: ideally, this should be an RPC call to the daemon API to correctly handle the leave request and reconfiguration.
func InitCluster(dataDir, machineName string, netPrefix netip.Prefix, users []*pb.User) error {
	var err error
	if machineName == "" {
		machineName, err = machine.NewRandomName()
		if err != nil {
			return fmt.Errorf("generate machine name: %w", err)
		}
	}
	privKey, pubKey, err := network.NewMachineKeys()
	if err != nil {
		return fmt.Errorf("generate machine keys: %w", err)
	}

	state := cluster.NewState(cluster.StatePath(dataDir))
	c := cluster.NewCluster(state, "")
	if err = c.SetNetwork(netPrefix); err != nil {
		return fmt.Errorf("set cluster network: %w", err)
	}

	// Use all routable addresses as endpoints.
	addrs, err := network.ListRoutableAddresses()
	if err != nil {
		return fmt.Errorf("list routable addresses: %w", err)
	}
	endpoints := make([]*pb.IPPort, len(addrs))
	for i, addr := range addrs {
		addrPort := netip.AddrPortFrom(addr, network.WireGuardPort)
		endpoints[i] = pb.NewIPPort(addrPort)
	}
	// Register the new machine in the cluster to populate the state and get its ID and subnet.
	req := &pb.AddMachineRequest{
		Name: machineName,
		Network: &pb.NetworkConfig{
			Endpoints: endpoints,
			PublicKey: pubKey,
		},
	}
	resp, err := c.AddMachine(context.Background(), req)
	if err != nil {
		return fmt.Errorf("add machine to cluster: %w", err)
	}

	m := resp.Machine
	subnet, err := m.Network.Subnet.ToPrefix()
	if err != nil {
		return err
	}
	manageIP, err := m.Network.ManagementIp.ToAddr()
	if err != nil {
		return err
	}
	mcfg := &machine.Config{
		ID:   m.Id,
		Name: m.Name,
		Network: &network.Config{
			Subnet:       subnet,
			ManagementIP: manageIP,
			PrivateKey:   privKey,
			PublicKey:    pubKey,
		},
	}

	// Add users to the cluster and build peers config from them.
	peers := make([]network.PeerConfig, len(users))
	for i, u := range users {
		if err = c.AddUser(u); err != nil {
			return fmt.Errorf("add user to cluster: %w", err)
		}
		userManageIP, uErr := u.Network.ManagementIp.ToAddr()
		if uErr != nil {
			return uErr
		}
		peers[i] = network.PeerConfig{
			ManagementIP: userManageIP,
			PublicKey:    u.Network.PublicKey,
		}
	}
	mcfg.Network.Peers = peers

	mcfg.SetPath(machine.ConfigPath(dataDir))
	if err = mcfg.Save(); err != nil {
		return fmt.Errorf("save machine config: %w", err)
	}

	fmt.Printf("Cluster initialised with machine %q\n", m.Name)
	return nil
}

type Daemon struct {
	config  *machine.Config
	cluster *cluster.Cluster
}

func New(dataDir string) (*Daemon, error) {
	cfg, err := machine.ParseConfig(machine.ConfigPath(dataDir))
	if err != nil {
		return nil, fmt.Errorf("load machine config: %w", err)
	}

	statePath := cluster.StatePath(dataDir)
	state := cluster.NewState(statePath)
	if err = state.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load cluster state: %w", err)
		}
		slog.Info("No cluster state found, creating a new one.", "path", statePath)
		if err = state.Save(); err != nil {
			return nil, fmt.Errorf("save cluster state: %w", err)
		}
	}

	apiAddr := net.JoinHostPort(cfg.Network.ManagementIP.String(), strconv.Itoa(machine.APIPort))
	c := cluster.NewCluster(state, apiAddr)

	return &Daemon{
		config:  cfg,
		cluster: c,
	}, nil
}

func (d *Daemon) Run(ctx context.Context) error {
	wgnet, err := network.NewWireGuardNetwork()
	if err != nil {
		return fmt.Errorf("create WireGuard network: %w", err)
	}
	if err = wgnet.Configure(*d.config.Network); err != nil {
		return fmt.Errorf("configure WireGuard network: %w", err)
	}
	//ctx, cancel := context.WithCancel(context.Background())
	//go wgnet.WatchEndpoints(ctx, peerEndpointChangeNotifier)

	//addrs, err := network.ListRoutableAddresses()
	//if err != nil {
	//	return err
	//}
	//fmt.Println("Addresses:", addrs)

	// Use an errgroup to coordinate error handling and graceful shutdown of multiple daemon components.
	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		slog.Info("Starting cluster.")
		if err = d.cluster.Run(); err != nil {
			return fmt.Errorf("cluster failed: %w", err)
		}
		return nil
	})
	errGroup.Go(func() error {
		if err = wgnet.Run(ctx); err != nil {
			return fmt.Errorf("WireGuard network failed: %w", err)
		}
		return nil
	})
	// Shutdown goroutine.
	errGroup.Go(func() error {
		<-ctx.Done()
		slog.Info("Stopping cluster.")
		d.cluster.Stop()
		slog.Info("Cluster stopped.")
		return nil
	})

	return errGroup.Wait()
}
