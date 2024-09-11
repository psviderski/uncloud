package cli

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/protobuf/types/known/emptypb"
	"net/netip"
	"uncloud/internal/cli/client"
	"uncloud/internal/cli/client/connector"
	"uncloud/internal/cli/config"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/secret"
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

func (cli *CLI) CreateCluster(name string, userPrivateKey secret.Secret) error {
	if _, ok := cli.config.Clusters[name]; ok {
		return fmt.Errorf("cluster %q already exists", name)
	}
	if userPrivateKey == nil {
		user, err := client.NewUser(nil)
		if err != nil {
			return fmt.Errorf("generate user: %w", err)
		}
		userPrivateKey = user.PrivateKey()
	}

	cli.config.Clusters[name] = &config.Cluster{
		Name:           name,
		UserPrivateKey: userPrivateKey,
	}
	return cli.config.Save()
}

func (cli *CLI) CreateDefaultCluster() error {
	if err := cli.CreateCluster(defaultClusterName, nil); err != nil {
		return err
	}
	return cli.SetCurrentCluster(defaultClusterName)
}

func (cli *CLI) SetCurrentCluster(name string) error {
	if _, ok := cli.config.Clusters[name]; !ok {
		return ErrNotFound
	}
	cli.config.CurrentCluster = name
	return cli.config.Save()
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
	sshConfig := &connector.SSHConnectorConfig{
		User:    remoteMachine.User,
		Host:    remoteMachine.Host,
		Port:    remoteMachine.Port,
		KeyPath: remoteMachine.KeyPath,
	}
	c, err := client.New(ctx, connector.NewSSHConnector(sshConfig))
	if err != nil {
		return fmt.Errorf("connect to remote machine: %w", err)
	}
	defer func() {
		_ = c.Close()
	}()

	if clusterName == "" {
		clusterName = defaultClusterName
	}
	if _, ok := cli.config.Clusters[clusterName]; ok {
		return fmt.Errorf("cluster %q already exists", clusterName)
	}
	user, err := client.NewUser(nil)
	if err != nil {
		return fmt.Errorf("generate cluster user: %w", err)
	}

	// TODO: download and install the latest uncloudd binary by running the install shell script from GitHub.
	//  For now upload the binary using scp manually.
	// TODO: Check if the machine is already provisioned and ask the user to reset it first.

	req := &pb.InitClusterRequest{
		MachineName: machineName,
		Network:     pb.NewIPPrefix(netPrefix),
		User: &pb.User{
			Network: &pb.NetworkConfig{
				ManagementIp: pb.NewIP(user.ManagementIP()),
				PublicKey:    user.PublicKey(),
			},
		},
	}
	resp, err := c.InitCluster(ctx, req)
	if err != nil {
		return fmt.Errorf("init cluster: %w", err)
	}
	fmt.Printf("Cluster %q initialised with machine %q\n", clusterName, resp.Machine.Name)

	if err = cli.CreateCluster(clusterName, user.PrivateKey()); err != nil {
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
}

func (cli *CLI) AddMachine(ctx context.Context, remoteMachine RemoteMachine, clusterName, machineName string) error {
	c, err := cli.ConnectCluster(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer func() {
		_ = c.Close()
	}()

	// Create a command executor and a machine API client over the SSH connection to the remote machine.
	sshClient, err := sshexec.Connect(remoteMachine.User, remoteMachine.Host, remoteMachine.Port, remoteMachine.KeyPath)
	if err != nil {
		return fmt.Errorf(
			"SSH login to remote machine %s: %w",
			config.NewSSHDestination(remoteMachine.User, remoteMachine.Host, remoteMachine.Port), err,
		)
	}
	machineExec := sshexec.NewRemote(sshClient)

	conn := connector.NewSSHConnectorFromClient(sshClient)
	machineClient, err := client.New(ctx, conn)
	if err != nil {
		return fmt.Errorf("connect to remote machine API: %w", err)
	}
	// TODO: Check if the machine is already provisioned using machineClient and ask the user to reset it first.

	// TODO: Download and install the latest uncloudd binary by running the install shell script from GitHub.
	//  For now upload the binary using scp manually.
	if _, err = machineExec.Run(ctx, "which uncloudd"); err != nil {
		return fmt.Errorf("uncloudd binary not found on the remote machine: %w", err)
	}

	resp, err := machineClient.Token(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("get remote machine token: %w", err)
	}
	token := resp.Token

	fmt.Println("Token:", token)

	//req := &pb.AddMachineRequest{
	//	Name: machineName,
	//	Network: &pb.NetworkConfig{
	//
	//	}
	//}
	//resp, err := c.AddMachine(ctx, req)
	//if err != nil {
	//	return fmt.Errorf("add machine to cluster: %w", err)
	//}

	// TODO:
	// --1. Establish a client connection to the remote machine.
	// --2. Check if the machine is already provisioned and ask the user to reset it first.
	// --3. Download and install the latest uncloudd binary by running the install shell script from GitHub.
	// 4. Request token from the remote machine.
	// 5. Add the machine to the cluster using its token and receive a configuration token.
	// 6. Request the machine to join the cluster using the configuration token.
	// 7. Save the machine's SSH connection details in the cluster config.

	//name, connCfg, err := c.AddMachine(ctx, machineName, user, host, port, sshKeyPath)
	//if err != nil {
	//	return fmt.Errorf("add machine to cluster %q: %w", cluster.Name(), err)
	//}
	//fmt.Printf("Machine %q added to cluster %q\n", name, cluster.Name())
	//
	//cli.config.Clusters[cluster.Name()].Connections = append(cli.config.Clusters[cluster.Name()].Connections, connCfg)
	//if err = cli.config.Save(); err != nil {
	//	return fmt.Errorf("save config: %w", err)
	//}

	return nil
}
