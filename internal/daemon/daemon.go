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
	c := cluster.NewCluster(&cluster.Config{}, state)
	if err = c.SetNetwork(netPrefix); err != nil {
		return fmt.Errorf("set cluster network: %w", err)
	}

	// Use all routable addresses as endpoints.
	addrs, err := network.ListRoutableIPs()
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
	mcfg := &machine.State{
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

	mcfg.SetPath(machine.StatePath(dataDir))
	if err = mcfg.Save(); err != nil {
		return fmt.Errorf("save machine config: %w", err)
	}

	fmt.Printf("Cluster initialised with machine %q\n", m.Name)
	return nil
}

type Daemon struct {
	state   *machine.State
	cluster *cluster.Server
}

func New(dataDir string) (*Daemon, error) {
	mstatePath := machine.StatePath(dataDir)
	mstate, err := machine.ParseState(mstatePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load machine config: %w", err)
		}
		// Generate an empty machine config with a new key pair.
		slog.Info("Machine config not found, creating a new one.", "path", mstatePath)
		privKey, pubKey, kErr := network.NewMachineKeys()
		if kErr != nil {
			return nil, fmt.Errorf("generate machine keys: %w", kErr)
		}
		slog.Info("Generated machine key pair.", "pubkey", pubKey)

		mstate = &machine.State{
			Network: &network.Config{
				PrivateKey: privKey,
				PublicKey:  pubKey,
			},
		}
		mstate.SetPath(mstatePath)
		if err = mstate.Save(); err != nil {
			return nil, fmt.Errorf("save machine config: %w", err)
		}
	}

	cstatePath := cluster.StatePath(dataDir)
	cstate := cluster.NewState(cstatePath)
	if err = cstate.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load cluster state: %w", err)
		}
		slog.Info("Cluster state not found, creating a new one.", "path", cstatePath)
		if err = cstate.Save(); err != nil {
			return nil, fmt.Errorf("save cluster state: %w", err)
		}
	}

	d := &Daemon{
		state: mstate,
	}
	if mstate.Network.IsConfigured() {
		config := &cluster.Config{
			APIAddr: net.JoinHostPort(mstate.Network.ManagementIP.String(), strconv.Itoa(machine.APIPort)),
		}
		d.cluster = cluster.NewCluster(config, cstate)
	}

	return d, nil
}

func (d *Daemon) Run(ctx context.Context) error {
	// Use an errgroup to coordinate error handling and graceful shutdown of multiple daemon components.
	errGroup, ctx := errgroup.WithContext(ctx)

	// Start the network only if it is configured.
	if d.state.Network.IsConfigured() {
		wgnet, err := network.NewWireGuardNetwork()
		if err != nil {
			return fmt.Errorf("create WireGuard network: %w", err)
		}
		if err = wgnet.Configure(*d.state.Network); err != nil {
			return fmt.Errorf("configure WireGuard network: %w", err)
		}

		//ctx, cancel := context.WithCancel(context.Background())
		//go wgnet.WatchEndpoints(ctx, peerEndpointChangeNotifier)

		//addrs, err := network.ListRoutableIPs()
		//if err != nil {
		//	return err
		//}
		//fmt.Println("Addresses:", addrs)

		errGroup.Go(func() error {
			if err = wgnet.Run(ctx); err != nil {
				return fmt.Errorf("WireGuard network failed: %w", err)
			}
			return nil
		})
	} else {
		slog.Info("Waiting for network configuration to start WireGuard network.")
	}

	if d.cluster != nil {
		errGroup.Go(func() error {
			slog.Info("Starting cluster.")
			if err := d.cluster.Run(); err != nil {
				return fmt.Errorf("cluster failed: %w", err)
			}
			return nil
		})
	}

	// Shutdown goroutine.
	errGroup.Go(func() error {
		<-ctx.Done()
		if d.cluster != nil {
			slog.Info("Stopping cluster.")
			d.cluster.Stop()
			slog.Info("Cluster server stopped.")
		}
		return nil
	})

	return errGroup.Wait()
}
