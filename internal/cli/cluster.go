package cli

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net/netip"
	"uncloud/internal/cli/config"
	"uncloud/internal/cmdexec"
	"uncloud/internal/machine"
	"uncloud/internal/secret"
)

var (
	ErrNotFound = errors.New("not found")
)

type Cluster struct {
	Name       string
	privateKey ed25519.PrivateKey

	config *config.Config
}

func (c *Cluster) toConfig() *config.Cluster {
	return &config.Cluster{
		Name:   c.Name,
		Secret: c.privateKey.Seed(),
	}
}

func (cli *CLI) CreateCluster(name string, privateKey ed25519.PrivateKey) (*Cluster, error) {
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

	c := &Cluster{
		Name:       name,
		privateKey: privateKey,
		config:     cli.config,
	}
	cli.config.Clusters[name] = c.toConfig()
	if err := cli.config.Save(); err != nil {
		return nil, err
	}

	return c, nil
}

func (cli *CLI) CreateDefaultCluster() (*Cluster, error) {
	c, err := cli.CreateCluster("default", nil)
	if err != nil {
		return nil, err
	}
	if err = cli.SetCurrentCluster(c.Name); err != nil {
		return nil, err
	}
	return c, nil
}

func (cli *CLI) GetCluster(name string) (*Cluster, error) {
	clusterCfg, ok := cli.config.Clusters[name]
	if !ok {
		return nil, ErrNotFound
	}
	privateKey, err := privateKeyFromSecret(clusterCfg.Secret)
	if err != nil {
		return nil, err
	}

	return &Cluster{
		Name:       name,
		privateKey: privateKey,
		config:     cli.config,
	}, nil
}

func (cli *CLI) GetCurrentCluster() (*Cluster, error) {
	return cli.GetCluster(cli.config.CurrentCluster)
}

func (cli *CLI) SetCurrentCluster(name string) error {
	if _, ok := cli.config.Clusters[name]; !ok {
		return ErrNotFound
	}
	cli.config.CurrentCluster = name
	return cli.config.Save()
}

func (cli *CLI) ListClusters() ([]*Cluster, error) {
	var clusters []*Cluster
	for name := range cli.config.Clusters {
		c, err := cli.GetCluster(name)
		if err != nil {
			return nil, fmt.Errorf("get cluster %q: %w", name, err)
		}
		clusters = append(clusters, c)
	}
	return clusters, nil
}

func (c *Cluster) AddMachine(ctx context.Context, name, user, host string, port int, sshKeyPath string) (string, error) {
	exec, err := cmdexec.Connect(user, host, port, sshKeyPath)
	if err != nil {
		return "", fmt.Errorf("SSH login to %s@%s:%d: %w", user, host, port, err)
	}
	defer func() {
		_ = exec.Close()
	}()

	mcfg, err := machine.NewBootstrapConfig(name, netip.Prefix{})
	if err != nil {
		return "", fmt.Errorf("generate machine bootstrap config: %w", err)
	}

	sudoPrefix := ""
	if user != "root" {
		sudoPrefix = "sudo"
	}

	_, err = exec.Run(ctx, cmdexec.QuoteCommand(sudoPrefix, "mkdir", "-p", machine.DefaultDataDir))
	if err != nil {
		return "", fmt.Errorf("create data directory %q: %w", machine.DefaultDataDir, err)
	}

	// TODO: Check if the machine is already provisioned and ask the user to reset it first.
	// Write the machine config to /var/lib/uncloud/machine.json by piping the JSON data to the file.
	mcfgData, err := mcfg.Encode()
	if err != nil {
		return "", fmt.Errorf("encode machine config: %w", err)
	}
	mcfgPath := cmdexec.Quote(machine.ConfigPath(machine.DefaultDataDir))
	createFileCmd := fmt.Sprintf("%s touch %s && %s chmod 600 %s", sudoPrefix, mcfgPath, sudoPrefix, mcfgPath)
	_, err = exec.Run(ctx, fmt.Sprintf("%s && echo %s | %s tee %s > /dev/null",
		createFileCmd, cmdexec.Quote(string(mcfgData)), sudoPrefix, mcfgPath))
	if err != nil {
		return "", fmt.Errorf("write machine config to %q: %w", mcfgPath, err)
	}
	fmt.Println("Machine config written to", mcfgPath)

	// TODO: download and install the latest uncloudd binary by running the install shell script from GitHub.
	//  For now upload the binary using scp manually.

	out, err := exec.Run(ctx, cmdexec.QuoteCommand(sudoPrefix, "systemctl", "restart", "uncloudd"))
	if err != nil {
		return "", fmt.Errorf("start uncloudd: %w: %s", err, out)
	}
	fmt.Println("uncloudd started")

	connConfig := config.MachineConnection{
		User:   user,
		Host:   host,
		Port:   port,
		SSHKey: sshKeyPath,
	}
	c.config.Clusters[c.Name].Machines = append(c.config.Clusters[c.Name].Machines, connConfig)
	if err = c.config.Save(); err != nil {
		return "", fmt.Errorf("save config: %w", err)
	}

	return mcfg.Name, nil
}

func privateKeyFromSecret(s secret.Secret) (ed25519.PrivateKey, error) {
	// Cluster secret in the config is a hex-encoded private key seed.
	if len(s) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid cluster secret length")
	}
	return ed25519.NewKeyFromSeed(s), nil
}
