package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
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
		machineName, err = cluster.NewRandomMachineName()
		if err != nil {
			return fmt.Errorf("generate machine name: %w", err)
		}
	}
	privKey, pubKey, err := network.NewMachineKeys()
	if err != nil {
		return fmt.Errorf("generate machine keys: %w", err)
	}

	state := cluster.NewState(cluster.StatePath(dataDir))
	c := cluster.NewServer(state)
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
	machine *machine.Machine
}

func New(dataDir string) (*Daemon, error) {
	config := &machine.Config{
		DataDir: dataDir,
	}
	mach, err := machine.NewMachine(config)
	if err != nil {
		return nil, fmt.Errorf("init machine: %w", err)
	}

	return &Daemon{
		machine: mach,
	}, nil
}

func (d *Daemon) Run(ctx context.Context) error {
	slog.Info("Starting machine.")
	return d.machine.Run(ctx)
}
