package client

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"net/netip"
	"uncloud/internal/cli/config"
	"uncloud/internal/machine"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/network"
	"uncloud/internal/secret"
	"uncloud/internal/sshexec"
)

type ClusterClient struct {
	config *config.Cluster

	connector Connector
	conn      *grpc.ClientConn
	client    pb.ClusterClient
}

func NewClusterClient(cfg *config.Cluster, connector Connector) (*ClusterClient, error) {
	if cfg.UserPrivateKey == nil {
		return nil, errors.New("cluster user_key must be set in the config")
	}
	return &ClusterClient{
		config:    cfg,
		connector: connector,
	}, nil
}

func (c *ClusterClient) Name() string {
	return c.config.Name
}

// HasMachines returns true if the cluster has at least one machine specified in the config.
func (c *ClusterClient) HasMachines() bool {
	return len(c.config.Machines) > 0
}

func (c *ClusterClient) User() (*User, error) {
	return NewUser(c.config.UserPrivateKey)
}

// TODO: implement Connect method that establishes a WireGuard tunnel to a cluster machine
//
//	and initializes an API client through it.
func (c *ClusterClient) connect(ctx context.Context) error {
	if c.conn != nil {
		return nil
	}
	if !c.HasMachines() {
		return errors.New("no machines specified in the cluster config")
	}

	conn, err := c.connector.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	c.conn = conn
	c.client = pb.NewClusterClient(conn)

	return nil
}

func (c *ClusterClient) Close() error {
	c.connector.Close()
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.client = nil
		return err
	}
	return nil
}

func (c *ClusterClient) AddMachine(
	ctx context.Context, name, user, host string, port int, sshKeyPath string,
) (string, config.MachineConnection, error) {
	client, err := sshexec.Connect(user, host, port, sshKeyPath)
	if err != nil {
		return "", config.MachineConnection{}, fmt.Errorf("SSH login to %s@%s:%d: %w", user, host, port, err)
	}
	exec := sshexec.NewRemote(client)
	defer func() {
		_ = exec.Close()
	}()

	// TODO: download and install the latest uncloudd binary by running the install shell script from GitHub.
	//  For now upload the binary using scp manually.
	// TODO: Check if the machine is already provisioned and ask the user to reset it first.
	// TODO: grab a list of routable IP addresses from the remote machine.

	addrs := []netip.Addr{}

	sudoPrefix := ""
	if user != "root" {
		sudoPrefix = "sudo"
	}

	if !c.HasMachines() {
		clusterUser, uErr := NewUser(c.config.UserPrivateKey)
		if uErr != nil {
			return "", config.MachineConnection{}, uErr
		}

		_, rErr := exec.Run(ctx, sshexec.QuoteCommand(
			sudoPrefix, "uncloud", "machine", "init",
			"--name", name,
			"--user-pubkey", clusterUser.PublicKey().String()))
		if rErr != nil {
			return "", config.MachineConnection{}, fmt.Errorf("initialise a new cluster on machine: %w", rErr)
		}
	} else {
		endpoints := make([]*pb.IPPort, len(addrs))
		for i, addr := range addrs {
			addrPort := netip.AddrPortFrom(addr, network.WireGuardPort)
			endpoints[i] = pb.NewIPPort(addrPort)
		}

		mcfg, err := c.newMachineConfig(ctx, name, addrs)
		if err != nil {
			return "", config.MachineConnection{}, fmt.Errorf("create machine config: %w", err)
		}

		_, err = exec.Run(ctx, sshexec.QuoteCommand(sudoPrefix, "mkdir", "-m", "700", "-p", machine.DefaultDataDir))
		if err != nil {
			return "", config.MachineConnection{}, fmt.Errorf("create data directory %q: %w", machine.DefaultDataDir, err)
		}

		// Write the machine config to /var/lib/uncloud/machine.json by piping the JSON data to the file.
		mcfgData, err := mcfg.Encode()
		if err != nil {
			return "", config.MachineConnection{}, fmt.Errorf("encode machine config: %w", err)
		}
		mcfgPath := sshexec.Quote(machine.StatePath(machine.DefaultDataDir))
		createFileCmd := fmt.Sprintf("%s touch %s && %s chmod 600 %s", sudoPrefix, mcfgPath, sudoPrefix, mcfgPath)
		_, err = exec.Run(ctx, fmt.Sprintf("%s && echo %s | %s tee %s > /dev/null",
			createFileCmd, sshexec.Quote(string(mcfgData)), sudoPrefix, mcfgPath))
		if err != nil {
			return "", config.MachineConnection{}, fmt.Errorf("write machine config to %q: %w", mcfgPath, err)
		}
		fmt.Println("Machine config written to", mcfgPath)
	}

	out, err := exec.Run(ctx, sshexec.QuoteCommand(sudoPrefix, "systemctl", "restart", "uncloudd"))
	if err != nil {
		return "", config.MachineConnection{}, fmt.Errorf("start uncloudd: %w: %s", err, out)
	}

	// Get the machine token to retrieve the public key from it.
	tokenOut, err := exec.Run(ctx, sshexec.QuoteCommand(sudoPrefix, "uncloud", "machine", "token"))
	if err != nil {
		return "", config.MachineConnection{}, fmt.Errorf("get machine token: %w: %s", err, out)
	}
	token, err := machine.ParseToken(tokenOut)
	if err != nil {
		return "", config.MachineConnection{}, fmt.Errorf("parse machine token: %w", err)
	}
	// TODO: replace command runs with sending gRPC request to the machine API via unix socket.
	name, err = exec.Run(ctx, fmt.Sprintf("%s cat %s | grep Name | cut -d'\"' -f4",
		sudoPrefix, machine.StatePath(machine.DefaultDataDir)))
	if err != nil {
		return "", config.MachineConnection{}, fmt.Errorf("get machine name: %w: %s", err, out)
	}

	connCfg := config.MachineConnection{
		Host:      host,
		PublicKey: token.PublicKey,
	}
	return name, connCfg, nil
}

// newMachineConfig creates a new machine config for a machine that is being added to the cluster.
// addrs is a list of routable IP addresses that the machine can be reached at.
func (c *ClusterClient) newMachineConfig(ctx context.Context, name string, addrs []netip.Addr) (*machine.State, error) {
	if !c.HasMachines() {
		// Create a bootstrap config for the first machine in the cluster.
		clusterUser, err := NewUser(c.config.UserPrivateKey)
		if err != nil {
			return nil, err
		}
		userPeerCfg := network.PeerConfig{
			ManagementIP: clusterUser.ManagementIP(),
			PublicKey:    clusterUser.PublicKey(),
		}
		mcfg, err := machine.NewBootstrapConfig(name, netip.Prefix{}, userPeerCfg)
		if err != nil {
			return nil, fmt.Errorf("generate machine bootstrap config: %w", err)
		}
		return mcfg, nil
	}

	// Create a config for a new machine in the cluster that has already been bootstrapped.
	privKey, pubKey, err := network.NewMachineKeys()
	if err != nil {
		return nil, fmt.Errorf("generate machine keys: %w", err)
	}
	endpoints := make([]netip.AddrPort, len(addrs))
	for i, addr := range addrs {
		// Hardcode the WireGuard port until it's required to be configurable.
		endpoints[i] = netip.AddrPortFrom(addr, network.WireGuardPort)
	}
	resp, err := c.registerNewMachine(ctx, name, endpoints, pubKey)
	if err != nil {
		return nil, fmt.Errorf("register new machine: %w", err)
	}
	minfo := resp.Machine
	fmt.Printf("Machine %q registered in the cluster with ID %q\n", minfo.Name, minfo.Id)

	//peers := make([]network.PeerConfig, len(resp.OtherMachines))
	//for i, pinfo := range resp.OtherMachines {
	//	peer := pinfo.Network
	//	if len(peer.Endpoints) == 0 {
	//		continue
	//	}
	//	peerSubnet, pErr := pinfo.Network.Subnet.ToPrefix()
	//	if pErr != nil {
	//		return nil, pErr
	//	}
	//	peerManageIP, pErr := pinfo.Network.ManagementIp.ToAddr()
	//	if pErr != nil {
	//		return nil, pErr
	//	}
	//	peerEndpoints := make([]netip.AddrPort, len(peer.Endpoints))
	//	for j, ep := range peer.Endpoints {
	//		if peerEndpoints[j], err = ep.ToAddrPort(); err != nil {
	//			return nil, pErr
	//		}
	//	}
	//	peers[i] = network.PeerConfig{
	//		Subnet:       &peerSubnet,
	//		ManagementIP: peerManageIP,
	//		// TODO: do not pick an endpoint and let the daemon do it.
	//		Endpoint:     &peerEndpoints[0],
	//		AllEndpoints: peerEndpoints,
	//		PublicKey:    pinfo.Network.PublicKey,
	//	}
	//}

	subnet, err := minfo.Network.Subnet.ToPrefix()
	if err != nil {
		return nil, err
	}
	manageIP, err := minfo.Network.ManagementIp.ToAddr()
	if err != nil {
		return nil, err
	}
	mcfg := &machine.State{
		ID:   minfo.Id,
		Name: minfo.Name,
		Network: &network.Config{
			Subnet:       subnet,
			ManagementIP: manageIP,
			PrivateKey:   privKey,
			PublicKey:    pubKey,
			//Peers:        peers,
		},
	}
	return mcfg, nil
}

func (c *ClusterClient) registerNewMachine(
	ctx context.Context, name string, endpoints []netip.AddrPort, publicKey secret.Secret,
) (*pb.AddMachineResponse, error) {
	if err := c.connect(ctx); err != nil {
		return nil, err
	}

	pbEndpoints := make([]*pb.IPPort, len(endpoints))
	for i, ep := range endpoints {
		pbEndpoints[i] = pb.NewIPPort(ep)
	}
	req := &pb.AddMachineRequest{
		Name: name,
		Network: &pb.NetworkConfig{
			Endpoints: pbEndpoints,
			PublicKey: publicKey,
		},
	}
	return c.client.AddMachine(ctx, req)
}

func privateKeyFromSecret(s secret.Secret) (ed25519.PrivateKey, error) {
	// Cluster secret in the config is a hex-encoded private key seed.
	if len(s) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid cluster secret length")
	}
	return ed25519.NewKeyFromSeed(s), nil
}
