package cli

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net/netip"
	"uncloud/internal/cli/client"
	"uncloud/internal/cli/client/connector"
	"uncloud/internal/cli/config"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/secret"
)

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

func (cli *CLI) CreateCluster(
	name string, privateKey ed25519.PrivateKey, userPrivateKey secret.Secret,
) (*client.ClusterClient, error) {
	if _, ok := cli.config.Clusters[name]; ok {
		return nil, fmt.Errorf("cluster %q already exists", name)
	}
	if privateKey == nil {
		var err error
		_, privateKey, err = ed25519.GenerateKey(nil)
		if err != nil {
			return nil, fmt.Errorf("generate cluster secret: %w", err)
		}
	}
	if userPrivateKey == nil {
		user, err := client.NewUser(nil)
		if err != nil {
			return nil, fmt.Errorf("generate user: %w", err)
		}
		userPrivateKey = user.PrivateKey()
	}

	cli.config.Clusters[name] = &config.Cluster{
		Name:           name,
		Secret:         privateKey.Seed(),
		UserPrivateKey: userPrivateKey,
	}
	if err := cli.config.Save(); err != nil {
		return nil, err
	}

	return cli.GetCluster(name)
}

func (cli *CLI) CreateDefaultCluster() (*client.ClusterClient, error) {
	c, err := cli.CreateCluster("default", nil, nil)
	if err != nil {
		return nil, err
	}
	if err = cli.SetCurrentCluster(c.Name()); err != nil {
		return nil, err
	}
	return c, nil
}

func (cli *CLI) GetCluster(name string) (*client.ClusterClient, error) {
	clusterCfg, ok := cli.config.Clusters[name]
	if !ok {
		return nil, ErrNotFound
	}
	clusterCfg.Name = name

	user, err := client.NewUser(clusterCfg.UserPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	wgConnector := connector.NewWireGuardConnector(user, clusterCfg.Machines)

	return client.NewClusterClient(clusterCfg, wgConnector)
}

func (cli *CLI) GetCurrentCluster() (*client.ClusterClient, error) {
	return cli.GetCluster(cli.config.CurrentCluster)
}

func (cli *CLI) SetCurrentCluster(name string) error {
	if _, ok := cli.config.Clusters[name]; !ok {
		return ErrNotFound
	}
	cli.config.CurrentCluster = name
	return cli.config.Save()
}

func (cli *CLI) ListClusters() ([]*client.ClusterClient, error) {
	var clusters []*client.ClusterClient
	for name := range cli.config.Clusters {
		c, err := cli.GetCluster(name)
		if err != nil {
			return nil, fmt.Errorf("get cluster %q: %w", name, err)
		}
		clusters = append(clusters, c)
	}
	return clusters, nil
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

	var cluster *client.ClusterClient
	if clusterName == "" {
		cluster, err = cli.CreateDefaultCluster()
	} else {
		cluster, err = cli.CreateCluster(clusterName, nil, nil)
	}
	if err != nil {
		return err
	}
	user, err := cluster.User()
	if err != nil {
		return fmt.Errorf("get cluster user: %w", err)
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
	fmt.Printf("Cluster %q initialised with machine %q\n", cluster.Name(), resp.Machine.Name)

	// Save the machine's SSH connection details in the cluster config.
	connCfg := config.MachineConnection{
		SSH: config.NewSSHDestination(remoteMachine.User, remoteMachine.Host, remoteMachine.Port),
	}
	cli.config.Clusters[cluster.Name()].Machines = append(cli.config.Clusters[cluster.Name()].Machines, connCfg)
	if err = cli.config.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

func (cli *CLI) AddMachine(
	ctx context.Context, clusterName, machineName, user, host string, port int, sshKeyPath string,
) error {
	var (
		cluster *client.ClusterClient
		err     error
	)
	if clusterName == "" {
		// If the cluster is not specified, use the current cluster. If there are no clusters, create a default one.
		cluster, err = cli.GetCurrentCluster()
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				// Do not create a default cluster if there are already clusters but the current cluster is not set.
				clusters, cErr := cli.ListClusters()
				if cErr != nil {
					return fmt.Errorf("list clusters: %w", cErr)
				}
				if len(clusters) > 0 {
					return errors.New(
						"the current cluster is not set in the Uncloud config. " +
							"Please specify a cluster with the --cluster flag or set current_cluster in the config",
					)
				}

				cluster, err = cli.CreateDefaultCluster()
				if err != nil {
					return fmt.Errorf("create default cluster: %w", err)
				}
				fmt.Printf("Created %q cluster\n", cluster.Name())
			} else {
				return fmt.Errorf("get current cluster: %w", err)
			}
		}
	} else {
		cluster, err = cli.GetCluster(clusterName)
		if err != nil {
			return fmt.Errorf("get cluster %q: %w", clusterName, err)
		}
	}
	defer func() {
		_ = cluster.Close()
	}()

	name, connCfg, err := cluster.AddMachine(ctx, machineName, user, host, port, sshKeyPath)
	if err != nil {
		return fmt.Errorf("add machine to cluster %q: %w", cluster.Name(), err)
	}
	fmt.Printf("Machine %q added to cluster %q\n", name, cluster.Name())

	cli.config.Clusters[cluster.Name()].Machines = append(cli.config.Clusters[cluster.Name()].Machines, connCfg)
	if err = cli.config.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	return nil
}
