package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/huh"
	"google.golang.org/protobuf/types/known/emptypb"
	"net/netip"
	"uncloud/internal/cli/client"
	"uncloud/internal/cli/client/connector"
	"uncloud/internal/cli/config"
	"uncloud/internal/machine"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/sshexec"
)

const defaultClusterName = "default"

var (
	ErrNotFound = errors.New("not found")
)

type CLI struct {
	config *config.Config
}

func New(configPath string) (*CLI, error) {
	cfg, err := config.NewFromFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read Uncloud config: %w", err)
	}
	return &CLI{
		config: cfg,
	}, nil
}

func (cli *CLI) CreateCluster(name string) error {
	if _, ok := cli.config.Clusters[name]; ok {
		return fmt.Errorf("cluster %q already exists", name)
	}
	cli.config.Clusters[name] = &config.Cluster{
		Name: name,
	}
	return cli.config.Save()
}

func (cli *CLI) SetCurrentCluster(name string) error {
	if _, ok := cli.config.Clusters[name]; !ok {
		return ErrNotFound
	}
	cli.config.CurrentCluster = name
	return cli.config.Save()
}

func (cli *CLI) ConnectCluster(ctx context.Context, clusterName string) (*client.Client, error) {
	if len(cli.config.Clusters) == 0 {
		return nil, errors.New(
			"no clusters found in the Uncloud config. " +
				"Please initialise a cluster with `uncloud machine init` first",
		)
	}
	if clusterName == "" {
		// If the cluster is not specified, use the current cluster if set.
		if cli.config.CurrentCluster == "" {
			return nil, errors.New(
				"the current cluster is not set in the Uncloud config. " +
					"Please specify a cluster with the --cluster flag or set current_cluster in the config",
			)
		}
		if _, ok := cli.config.Clusters[cli.config.CurrentCluster]; !ok {
			return nil, fmt.Errorf(
				"current cluster %q not found in the config. "+
					"Please specify a cluster with the --cluster flag or update current_cluster in the config",
				cli.config.CurrentCluster,
			)
		}
		clusterName = cli.config.CurrentCluster
	}

	cfg, ok := cli.config.Clusters[clusterName]
	if !ok {
		return nil, fmt.Errorf("cluster %q not found in the config", clusterName)
	}
	if len(cfg.Connections) == 0 {
		return nil, fmt.Errorf("no connection configurations found for cluster %q in the config", clusterName)
	}

	// TODO: iterate over all connections and try to connect to the cluster using the first successful connection.
	conn := cfg.Connections[0]
	if conn.SSH != "" {
		user, host, port, err := conn.SSH.Parse()
		if err != nil {
			return nil, fmt.Errorf("parse SSH connection %q: %w", conn.SSH, err)
		}
		sshConfig := &connector.SSHConnectorConfig{
			User: user,
			Host: host,
			Port: port,
		}
		return client.New(ctx, connector.NewSSHConnector(sshConfig))
	} else if conn.TCP.IsValid() {
		return client.New(ctx, connector.NewTCPConnector(conn.TCP))
	}
	return nil, errors.New("no valid connection configuration found for the cluster")
}

func (cli *CLI) InitCluster(
	ctx context.Context, remoteMachine *RemoteMachine, clusterName, machineName string, netPrefix netip.Prefix,
) error {
	if remoteMachine != nil {
		return cli.initRemoteMachine(ctx, *remoteMachine, clusterName, machineName, netPrefix)
	}
	// TODO: implement local machine initialisation
	return fmt.Errorf("local machine initialisation is not implemented yet")
}

func (cli *CLI) initRemoteMachine(
	ctx context.Context, remoteMachine RemoteMachine, clusterName, machineName string, netPrefix netip.Prefix,
) error {
	if clusterName == "" {
		clusterName = defaultClusterName
	}
	if _, ok := cli.config.Clusters[clusterName]; ok {
		return fmt.Errorf("cluster %q already exists", clusterName)
	}

	machineClient, err := cli.provisionRemoteMachine(ctx, remoteMachine)
	if err != nil {
		return err
	}
	defer machineClient.Close()

	// Check if the machine is already initialised as a cluster member and prompt the user to reset it first.
	minfo, err := machineClient.Inspect(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("inspect machine: %w", err)
	}
	if minfo.Id != "" {
		if err = cli.promptResetMachine(); err != nil {
			return err
		}
	}

	req := &pb.InitClusterRequest{
		MachineName: machineName,
		Network:     pb.NewIPPrefix(netPrefix),
	}
	resp, err := machineClient.InitCluster(ctx, req)
	if err != nil {
		return fmt.Errorf("init cluster: %w", err)
	}
	fmt.Printf("Cluster %q initialised with machine %q\n", clusterName, resp.Machine.Name)

	if err = cli.CreateCluster(clusterName); err != nil {
		return fmt.Errorf("save cluster to config: %w", err)
	}
	// Set the current cluster to the just created one if it is the only cluster in the config.
	if len(cli.config.Clusters) == 1 {
		if err = cli.SetCurrentCluster(clusterName); err != nil {
			return fmt.Errorf("set current cluster: %w", err)
		}
	}
	// Save the machine's SSH connection details in the cluster config.
	connCfg := config.MachineConnection{
		SSH: config.NewSSHDestination(remoteMachine.User, remoteMachine.Host, remoteMachine.Port),
	}
	cli.config.Clusters[clusterName].Connections = append(cli.config.Clusters[clusterName].Connections, connCfg)
	if err = cli.config.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

func (cli *CLI) AddMachine(ctx context.Context, remoteMachine RemoteMachine, clusterName, machineName string) error {
	c, err := cli.ConnectCluster(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer func() {
		_ = c.Close()
	}()

	machineClient, err := cli.provisionRemoteMachine(ctx, remoteMachine)
	if err != nil {
		return err
	}
	defer machineClient.Close()

	// Check if the machine is already initialised as a cluster member and prompt the user to reset it first.
	minfo, err := machineClient.Inspect(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("inspect machine: %w", err)
	}
	if minfo.Id != "" {
		if err = cli.promptResetMachine(); err != nil {
			return err
		}
	}

	tokenResp, err := machineClient.Token(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("get remote machine token: %w", err)
	}
	token, err := machine.ParseToken(tokenResp.Token)
	if err != nil {
		return fmt.Errorf("parse remote machine token: %w", err)
	}

	// Register the machine in the cluster using its public key and endpoints from the token.
	endpoints := make([]*pb.IPPort, len(token.Endpoints))
	for i, addrPort := range token.Endpoints {
		endpoints[i] = pb.NewIPPort(addrPort)
	}
	addReq := &pb.AddMachineRequest{
		Name: machineName,
		Network: &pb.NetworkConfig{
			Endpoints: endpoints,
			PublicKey: token.PublicKey,
		},
	}
	addResp, err := c.AddMachine(ctx, addReq)
	if err != nil {
		return fmt.Errorf("add machine to cluster: %w", err)
	}

	// List other machines in the cluster to include them in the join request.
	machines, err := c.ListMachines(ctx)
	if err != nil {
		return fmt.Errorf("list cluster machines: %w", err)
	}
	otherMachines := make([]*pb.MachineInfo, 0, len(machines)-1)
	for _, m := range machines {
		if m.Machine.Id != addResp.Machine.Id {
			otherMachines = append(otherMachines, m.Machine)
		}
	}

	// Configure the remote machine to join the cluster.
	joinReq := &pb.JoinClusterRequest{
		Machine:       addResp.Machine,
		OtherMachines: otherMachines,
	}
	if _, err = machineClient.JoinCluster(ctx, joinReq); err != nil {
		return fmt.Errorf("join cluster: %w", err)
	}

	fmt.Printf("Machine %q added to cluster\n", addResp.Machine.Name)

	// Save the machine's SSH connection details in the cluster config.
	connCfg := config.MachineConnection{
		SSH: config.NewSSHDestination(remoteMachine.User, remoteMachine.Host, remoteMachine.Port),
	}
	if clusterName == "" {
		clusterName = cli.config.CurrentCluster
	}
	cli.config.Clusters[clusterName].Connections = append(cli.config.Clusters[clusterName].Connections, connCfg)
	if err = cli.config.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	return nil
}

// provisionRemoteMachine installs the Uncloud daemon and dependencies on the remote machine over SSH and returns
// a machine API client to interact with the machine. The client should be closed after use by the caller.
func (cli *CLI) provisionRemoteMachine(ctx context.Context, remoteMachine RemoteMachine) (*client.Client, error) {
	// Provision the remote machine by installing the Uncloud daemon and dependencies over SSH.
	sshClient, err := sshexec.Connect(remoteMachine.User, remoteMachine.Host, remoteMachine.Port, remoteMachine.KeyPath)
	if err != nil {
		return nil, fmt.Errorf(
			"SSH login to remote machine %s: %w",
			config.NewSSHDestination(remoteMachine.User, remoteMachine.Host, remoteMachine.Port), err,
		)
	}
	exec := sshexec.NewRemote(sshClient)
	// Install and run the Uncloud daemon and dependencies on the remote machine.
	if err = provisionMachine(ctx, exec); err != nil {
		return nil, fmt.Errorf("provision machine: %w", err)
	}

	var machineClient *client.Client
	if remoteMachine.User == "root" {
		// Create a machine API client over the established SSH connection to the remote machine.
		machineClient, err = client.New(ctx, connector.NewSSHConnectorFromClient(sshClient))
	} else {
		// Since the user is not root, we need to establish a new SSH connection to make the user's addition
		// to the uncloud group effective, thus allowing access to the Uncloud daemon Unix socket.
		sshConfig := &connector.SSHConnectorConfig{
			User:    remoteMachine.User,
			Host:    remoteMachine.Host,
			Port:    remoteMachine.Port,
			KeyPath: remoteMachine.KeyPath,
		}
		machineClient, err = client.New(ctx, connector.NewSSHConnector(sshConfig))
	}
	if err != nil {
		return nil, fmt.Errorf("connect to remote machine: %w", err)
	}
	return machineClient, nil
}

func (cli *CLI) promptResetMachine() error {
	var confirm bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(
					"The remote machine is already initialised as a cluster member. Do you want to reset it first?",
				).
				Affirmative("Yes!").
				Negative("No").
				Value(&confirm),
		),
	)
	if err := form.Run(); err != nil {
		return fmt.Errorf("prompt user to confirm: %w", err)
	}

	if !confirm {
		return fmt.Errorf("remote machine is already initialised as a cluster member")
	}
	// TODO: implement resetting the remote machine.
	return fmt.Errorf("resetting the remote machine is not implemented yet")
}
