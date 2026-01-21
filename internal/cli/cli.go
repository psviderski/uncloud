package cli

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"slices"

	"github.com/docker/cli/cli/streams"
	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/psviderski/uncloud/internal/machine"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/sshexec"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/connector"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	// DefaultSSHKeyPath is the fallback location for the SSH private key when provisioning remote machines.
	// Used when no key is explicitly provided and SSH agent authentication fails.
	DefaultSSHKeyPath  = "~/.ssh/id_ed25519"
	DefaultContextName = "default"
)

type CLI struct {
	Config          *config.Config
	conn            *config.MachineConnection
	contextOverride string
}

// New creates a new CLI instance with the given config path or remote machine connection.
// If the connection is provided, the config is ignored for all operations which is useful for interacting with
// a cluster without creating a config.
// If a non-empty context name is given, it will override the current default.
func New(configPath string, conn *config.MachineConnection, contextName string) (*CLI, error) {
	if conn != nil {
		return &CLI{conn: conn}, nil
	}

	cfg, err := config.NewFromFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read Uncloud config: %w", err)
	}

	return &CLI{
		Config:          cfg,
		contextOverride: contextName,
	}, nil
}

func (cli *CLI) CreateContext(name string) error {
	if _, ok := cli.Config.Contexts[name]; ok {
		return fmt.Errorf("context '%s' already exists", name)
	}
	cli.Config.Contexts[name] = &config.Context{
		Name: name,
	}
	return cli.Config.Save()
}

func (cli *CLI) SetCurrentContext(name string) error {
	if _, ok := cli.Config.Contexts[name]; !ok {
		return api.ErrNotFound
	}
	cli.Config.CurrentContext = name
	return cli.Config.Save()
}

func (cli *CLI) GetContextOverrideOrCurrent() string {
	contextName := cli.contextOverride
	if contextName == "" {
		contextName = cli.Config.CurrentContext
	}
	return contextName
}

// ConnectCluster connects to a cluster using the context override or the current context if not specified.
// If the CLI was initialised with a machine connection, the config is ignored and the connection is used instead.
func (cli *CLI) ConnectCluster(ctx context.Context) (*client.Client, error) {
	return cli.ConnectClusterWithOptions(ctx, ConnectOptions{
		// Default to showing progress for CLI usage.
		ShowProgress: true,
	})
}

// ConnectClusterWithOptions connects to a cluster with the given options.
// If the CLI was initialised with a machine connection, the config is ignored and the connection is used instead.
// If the CLI has an override context, it is used instead of the current default.
// Options are useful when using the CLI as a library where you may want to disable visual feedback.
func (cli *CLI) ConnectClusterWithOptions(ctx context.Context, opts ConnectOptions) (*client.Client, error) {
	if cli.conn != nil {
		return ConnectCluster(ctx, *cli.conn, opts)
	}

	if len(cli.Config.Contexts) == 0 {
		return nil, fmt.Errorf(
			"no cluster contexts found in the Uncloud config (%s). "+
				"Please initialise a cluster with 'uncloud machine init' first",
			cli.Config.Path(),
		)
	}

	contextName := cli.contextOverride
	if contextName == "" {
		// If the cluster is not specified, use the current cluster if set.
		if cli.Config.CurrentContext == "" {
			return nil, fmt.Errorf(
				"the current cluster context is not set in the Uncloud config (%s). "+
					"Please specify the context with the '--context' flag or set 'current_context' in the config",
				cli.Config.Path(),
			)
		}
		if _, ok := cli.Config.Contexts[cli.Config.CurrentContext]; !ok {
			return nil, fmt.Errorf(
				"current cluster context '%s' not found in the Uncloud config (%s). "+
					"Please specify the context with the '--context' flag or update 'current_context' in the config",
				cli.Config.CurrentContext,
				cli.Config.Path(),
			)
		}
		contextName = cli.Config.CurrentContext
	}

	cfg, ok := cli.Config.Contexts[contextName]
	if !ok {
		return nil, fmt.Errorf("cluster context '%s' not found in the Uncloud config (%s)",
			contextName, cli.Config.Path())
	}
	if len(cfg.Connections) == 0 {
		return nil, fmt.Errorf(
			"no connection configurations found for cluster context '%s' in the Uncloud config (%s)",
			contextName, cli.Config.Path(),
		)
	}

	// Try each connection in order until one succeeds.
	var lastErr error
	for _, conn := range cfg.Connections {
		c, err := ConnectCluster(ctx, conn, opts)
		if err == nil {
			return c, nil
		}

		lastErr = err
	}

	return nil, fmt.Errorf("failed to connect to cluster context '%s': "+
		"all connections (%d) in the Uncloud config (%s) failed; last error: %w",
		contextName, len(cfg.Connections), cli.Config.Path(), lastErr)
}

type InitClusterOptions struct {
	Context       string
	MachineName   string
	Network       netip.Prefix
	PublicIP      *netip.Addr
	RemoteMachine *RemoteMachine
	SkipInstall   bool
	Version       string
	AutoConfirm   bool
}

// InitCluster initialises a new cluster on a remote machine and returns a client to interact with the cluster.
// The client should be closed after use by the caller.
func (cli *CLI) InitCluster(ctx context.Context, opts InitClusterOptions) (*client.Client, error) {
	if opts.RemoteMachine != nil {
		return cli.initRemoteMachine(ctx, opts)
	}
	// TODO: implement local machine initialisation
	return nil, fmt.Errorf("local machine initialisation is not implemented yet. Please specify a remote machine")
}

func (cli *CLI) initRemoteMachine(ctx context.Context, opts InitClusterOptions) (*client.Client, error) {
	contextName, err := cli.newContextName(opts.Context)
	if err != nil {
		return nil, err
	}

	machineClient, err := provisionOrConnectRemoteMachine(ctx, opts.RemoteMachine, opts.SkipInstall, opts.Version)
	if err != nil {
		return nil, err
	}
	// Ensure machineClient is closed on error.
	defer func() {
		if err != nil {
			machineClient.Close()
		}
	}()

	// Check if the machine is already initialised as a cluster member and prompt the user to reset it first.
	minfo, err := machineClient.Inspect(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("inspect machine: %w", err)
	}
	if minfo.Id != "" {
		if !opts.AutoConfirm {
			if err = promptResetMachine(); err != nil {
				return nil, err
			}
		}
		if err = resetAndWaitMachine(ctx, machineClient.MachineClient); err != nil {
			return nil, err
		}
	}

	// Check machine meets all necessary system requirements before proceeding.
	checkResp, err := machineClient.CheckPrerequisites(ctx, &emptypb.Empty{})
	// TODO(lhf): remove Unimplemented check when v0.9.0 is released.
	if err != nil {
		if status.Convert(err).Code() != codes.Unimplemented {
			return nil, fmt.Errorf("check machine prerequisites: %w", err)
		}
	} else if !checkResp.Satisfied {
		return nil, fmt.Errorf("machine prerequisites not satisfied: %s", checkResp.Error)
	}

	req := &pb.InitClusterRequest{
		MachineName: opts.MachineName,
		Network:     pb.NewIPPrefix(opts.Network),
	}
	if opts.PublicIP != nil {
		if opts.PublicIP.IsValid() {
			req.PublicIpConfig = &pb.InitClusterRequest_PublicIp{PublicIp: pb.NewIP(*opts.PublicIP)}
		} else {
			// Invalid or in other words zero IP means to automatically detect the public IP.
			req.PublicIpConfig = &pb.InitClusterRequest_PublicIpAuto{PublicIpAuto: true}
		}
	}

	resp, err := machineClient.InitCluster(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("init cluster: %w", err)
	}
	fmt.Printf("Cluster initialised with machine '%s' and saved as context '%s' in your local config (%s)\n",
		resp.Machine.Name, contextName, cli.Config.Path())
	if err = cli.CreateContext(contextName); err != nil {
		return nil, fmt.Errorf("save cluster context to config: %w", err)
	}
	if err = cli.SetCurrentContext(contextName); err != nil {
		return nil, fmt.Errorf("set current cluster context: %w", err)
	}
	fmt.Printf("Current cluster context is now '%s'.\n", contextName)

	// Save the machine's SSH connection details in the context config.
	connCfg := config.MachineConnection{
		SSHKeyFile: opts.RemoteMachine.KeyPath,
		MachineID:  resp.Machine.Id,
	}
	if opts.RemoteMachine.UseSSHCLI {
		connCfg.SSHCLI = config.NewSSHDestination(
			opts.RemoteMachine.User,
			opts.RemoteMachine.Host,
			opts.RemoteMachine.Port,
		)
	} else {
		connCfg.SSH = config.NewSSHDestination(
			opts.RemoteMachine.User,
			opts.RemoteMachine.Host,
			opts.RemoteMachine.Port,
		)
	}
	cli.Config.Contexts[contextName].Connections = append(cli.Config.Contexts[contextName].Connections, connCfg)
	if err = cli.Config.Save(); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}
	return machineClient, nil
}

// newContextName returns a unique name for a new cluster context. If the provided name is not DefaultContextName,
// and it's already taken, an error is returned. If the name is not provided or is DefaultContextName, the first
// available name "default[-N]" is returned.
func (cli *CLI) newContextName(name string) (string, error) {
	if name == "" {
		name = DefaultContextName
	}

	if _, exists := cli.Config.Contexts[name]; !exists {
		return name, nil
	}

	// If non-default context already exists, error out.
	if name != DefaultContextName {
		return "", fmt.Errorf("cluster context '%s' already exists", name)
	}

	// The default context already exists, generate a numbered suffix to make it unique.
	for i := 1; ; i++ {
		name = fmt.Sprintf("%s-%d", DefaultContextName, i)
		if _, exists := cli.Config.Contexts[name]; !exists {
			return name, nil
		}
	}
}

type AddMachineOptions struct {
	MachineName   string
	PublicIP      *netip.Addr
	RemoteMachine *RemoteMachine
	SkipInstall   bool
	Version       string
	AutoConfirm   bool
}

// AddMachine provisions a remote machine and adds it to the cluster. It returns a cluster client and a machine client.
// The cluster client is connected to the existing machine in the cluster. It was used to add the new machine to the
// cluster. The machine client is connected to the new machine and can be used to interact with it.
// Both client should be closed after use by the caller.
func (cli *CLI) AddMachine(ctx context.Context, opts AddMachineOptions) (*client.Client, *client.Client, error) {
	contextName := cli.GetContextOverrideOrCurrent()
	c, err := cli.ConnectCluster(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to cluster (context '%s'): %w", contextName, err)
	}
	defer func() {
		if err != nil {
			c.Close()
		}
	}()

	machineClient, err := provisionOrConnectRemoteMachine(ctx, opts.RemoteMachine, opts.SkipInstall, opts.Version)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			machineClient.Close()
		}
	}()

	// Check if the machine is already initialised as a cluster member and prompt the user to reset it first.
	// TODO: refactor to use client.InspectMachine.
	minfo, err := machineClient.Inspect(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, nil, fmt.Errorf("inspect machine: %w", err)
	}
	if minfo.Id != "" {
		// Check if the machine is already a member of this cluster.
		machines, err := c.ListMachines(ctx, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("list cluster machines: %w", err)
		}
		if slices.ContainsFunc(machines, func(m *pb.MachineMember) bool {
			return m.Machine.Id == minfo.Id
		}) {
			return nil, nil, fmt.Errorf("machine is already a member of this cluster (%s)", minfo.Name)
		}

		if !opts.AutoConfirm {
			if err = promptResetMachine(); err != nil {
				return nil, nil, err
			}
		}
		if err = resetAndWaitMachine(ctx, machineClient.MachineClient); err != nil {
			return nil, nil, err
		}
	}

	// Check machine meets all necessary system requirements before proceeding.
	checkResp, err := machineClient.CheckPrerequisites(ctx, &emptypb.Empty{})
	// TODO(lhf): remove Unimplemented check when v0.9.0 is released.
	if err != nil {
		if status.Convert(err).Code() != codes.Unimplemented {
			return nil, nil, fmt.Errorf("check machine prerequisites: %w", err)
		}
	} else if !checkResp.Satisfied {
		return nil, nil, fmt.Errorf("machine prerequisites not satisfied: %s", checkResp.Error)
	}

	tokenResp, err := machineClient.Token(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, nil, fmt.Errorf("get remote machine token: %w", err)
	}
	token, err := machine.ParseToken(tokenResp.Token)
	if err != nil {
		return nil, nil, fmt.Errorf("parse remote machine token: %w", err)
	}

	// Register the machine in the cluster using its public key and endpoints from the token.
	endpoints := make([]*pb.IPPort, len(token.Endpoints))
	for i, addrPort := range token.Endpoints {
		endpoints[i] = pb.NewIPPort(addrPort)
	}
	addReq := &pb.AddMachineRequest{
		Name: opts.MachineName,
		Network: &pb.NetworkConfig{
			Endpoints: endpoints,
			PublicKey: token.PublicKey,
		},
	}
	if opts.PublicIP != nil {
		if opts.PublicIP.IsValid() {
			addReq.PublicIp = pb.NewIP(*opts.PublicIP)
		} else if token.PublicIP.IsValid() {
			// Invalid or in other words zero IP means to use an automatically detected public IP from the token.
			addReq.PublicIp = pb.NewIP(token.PublicIP)
		}
	}

	addResp, err := c.AddMachine(ctx, addReq)
	if err != nil {
		return nil, nil, fmt.Errorf("add machine to cluster (context '%s'): %w", contextName, err)
	}

	// Get the current store DB version from the cluster to pass to the join request.
	var storeDBVersion int64
	inspectResp, err := c.MachineClient.InspectMachine(ctx, &emptypb.Empty{})
	if err != nil {
		// TODO(lhf): remove Unimplemented check when v0.17.0 is released.
		if status.Convert(err).Code() != codes.Unimplemented {
			return nil, nil, fmt.Errorf("inspect current cluster machine: %w", err)
		}
	} else {
		storeDBVersion = inspectResp.Machines[0].StoreDbVersion
	}

	// Get the most up-to-date list of other machines in the cluster to include them in the join request.
	machines, err := c.ListMachines(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("list cluster machines: %w", err)
	}
	otherMachines := make([]*pb.MachineInfo, 0, len(machines)-1)
	for _, m := range machines {
		if m.Machine.Id != addResp.Machine.Id {
			otherMachines = append(otherMachines, m.Machine)
		}
	}

	// Configure the remote machine to join the cluster.
	joinReq := &pb.JoinClusterRequest{
		Machine:           addResp.Machine,
		OtherMachines:     otherMachines,
		MinStoreDbVersion: storeDBVersion,
	}
	if _, err = machineClient.JoinCluster(ctx, joinReq); err != nil {
		return nil, nil, fmt.Errorf("join cluster: %w", err)
	}

	// TODO: fix empty context name when using the current context (contextName == "").
	fmt.Printf("Machine '%s' added to the cluster (context '%s').\n", addResp.Machine.Name, contextName)

	// Save the machine's SSH connection details in the context config.
	connCfg := config.MachineConnection{
		SSHKeyFile: opts.RemoteMachine.KeyPath,
		MachineID:  addResp.Machine.Id,
	}
	if opts.RemoteMachine.UseSSHCLI {
		connCfg.SSHCLI = config.NewSSHDestination(
			opts.RemoteMachine.User,
			opts.RemoteMachine.Host,
			opts.RemoteMachine.Port,
		)
	} else {
		connCfg.SSH = config.NewSSHDestination(
			opts.RemoteMachine.User,
			opts.RemoteMachine.Host,
			opts.RemoteMachine.Port,
		)
	}
	cli.Config.Contexts[contextName].Connections = append(cli.Config.Contexts[contextName].Connections, connCfg)
	if err = cli.Config.Save(); err != nil {
		return nil, nil, fmt.Errorf("save config: %w", err)
	}

	return c, machineClient, nil
}

// provisionOrConnectRemoteMachine installs the Uncloud daemon and dependencies on the remote machine over SSH and
// returns a machine API client to interact with the machine. The client should be closed after use by the caller.
// The version parameter specifies the version of the Uncloud daemon to install. If empty, the latest version is used.
// If skipInstall is true, the installation step is skipped, and it is assumed that the Uncloud daemon and dependencies
// are already installed and running.
// The remoteMachine.SSHKeyPath could be updated to the default SSH key path if it is not set and the SSH agent
// authentication fails.
func provisionOrConnectRemoteMachine(
	ctx context.Context, remoteMachine *RemoteMachine, skipInstall bool, version string,
) (*client.Client, error) {
	// Use SSH CLI
	if remoteMachine.UseSSHCLI {
		exec := sshexec.NewSSHCLIRemote(
			remoteMachine.User,
			remoteMachine.Host,
			remoteMachine.Port,
			remoteMachine.KeyPath,
		)

		if !skipInstall {
			if err := provisionMachine(ctx, exec, version); err != nil {
				return nil, fmt.Errorf("provision machine: %w", err)
			}
		}

		sshConfig := &connector.SSHConnectorConfig{
			User:    remoteMachine.User,
			Host:    remoteMachine.Host,
			Port:    remoteMachine.Port,
			KeyPath: remoteMachine.KeyPath,
		}
		machineClient, err := client.New(ctx, connector.NewSSHCLIConnector(sshConfig))
		if err != nil {
			return nil, fmt.Errorf("connect to remote machine: %w", err)
		}
		return machineClient, nil
	}

	// Use Go SSH
	sshClient, err := sshexec.Connect(remoteMachine.User, remoteMachine.Host, remoteMachine.Port, remoteMachine.KeyPath)
	// If the SSH connection using SSH agent fails and no key path is provided, try to use the default SSH key.
	if err != nil && remoteMachine.KeyPath == "" {
		remoteMachine.KeyPath = DefaultSSHKeyPath
		sshClient, err = sshexec.Connect(
			remoteMachine.User, remoteMachine.Host, remoteMachine.Port, remoteMachine.KeyPath,
		)
	}
	if err != nil {
		return nil, fmt.Errorf(
			"SSH login to remote machine %s: %w",
			config.NewSSHDestination(remoteMachine.User, remoteMachine.Host, remoteMachine.Port), err,
		)
	}

	if !skipInstall {
		// Provision the remote machine by installing the Uncloud daemon and dependencies over SSH.
		exec := sshexec.NewRemote(sshClient)
		if err = provisionMachine(ctx, exec, version); err != nil {
			return nil, fmt.Errorf("provision machine: %w", err)
		}
	}

	var machineClient *client.Client
	if remoteMachine.User == "root" || skipInstall {
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

// ProgressOut returns an output stream for progress writer.
func (cli *CLI) ProgressOut() *streams.Out {
	return streams.NewOut(os.Stdout)
}
