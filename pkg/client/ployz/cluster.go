package client

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/cmd/uncloud/caddy"
	"github.com/psviderski/uncloud/internal/cli"
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

type AddOptions struct {
	Destination string
	Name        string
	PublicIP    string
	SSHKey      string
	Version     string
	SocketPath  string
}

const (
	// PublicIPNone is the value used to indicate removal of public IP.
	PublicIPNone = "none"

	installScriptURL = "https://raw.githubusercontent.com/psviderski/uncloud/refs/heads/main/scripts/install.sh"
	rootUser         = "root"
	defaultSSHPort   = 22
)

func AddMachine(ctx context.Context, opts AddOptions) error {
	socketPath := opts.SocketPath
	if socketPath == "" {
		socketPath = machine.DefaultUncloudSockPath
	}

	// Connect to the existing cluster via Unix socket.
	clusterClient, err := client.New(ctx, connector.NewUnixConnector(socketPath))
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	// Parse SSH destination.
	user, host, port, err := parseSSHDestination(opts.Destination)
	if err != nil {
		return fmt.Errorf("parse remote machine: %w", err)
	}

	// Provision and connect to the remote machine.
	machineClient, err := provisionAndConnect(ctx, user, host, port, opts.SSHKey, opts.Version)
	if err != nil {
		return fmt.Errorf("provision machine: %w", err)
	}
	defer machineClient.Close()

	// Check if the machine is already initialised as a cluster member.
	minfo, err := machineClient.Inspect(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("inspect machine: %w", err)
	}
	if minfo.Id != "" {
		// Check if the machine is already a member of this cluster.
		machines, err := clusterClient.ListMachines(ctx, nil)
		if err != nil {
			return fmt.Errorf("list cluster machines: %w", err)
		}
		if slices.ContainsFunc(machines, func(m *pb.MachineMember) bool {
			return m.Machine.Id == minfo.Id
		}) {
			return fmt.Errorf("machine is already a member of this cluster (%s)", minfo.Name)
		}

		// Auto-confirm reset for programmatic usage.
		if err = resetAndWaitMachine(ctx, machineClient.MachineClient); err != nil {
			return err
		}
	}

	// Check machine meets all necessary system requirements before proceeding.
	checkResp, err := machineClient.MachineClient.CheckPrerequisites(ctx, &emptypb.Empty{})
	if err != nil {
		if status.Convert(err).Code() != codes.Unimplemented {
			return fmt.Errorf("check machine prerequisites: %w", err)
		}
	} else if !checkResp.Satisfied {
		return fmt.Errorf("machine prerequisites not satisfied: %s", checkResp.Error)
	}

	// Get machine token from the new machine.
	tokenResp, err := machineClient.MachineClient.Token(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("get remote machine token: %w", err)
	}
	token, err := machine.ParseToken(tokenResp.Token)
	if err != nil {
		return fmt.Errorf("parse remote machine token: %w", err)
	}

	// Parse public IP option.
	var publicIPProto *pb.IP
	switch opts.PublicIP {
	case "auto":
		if token.PublicIP.IsValid() {
			publicIPProto = pb.NewIP(token.PublicIP)
		}
	case "", PublicIPNone:
		publicIPProto = nil
	default:
		ip, err := netip.ParseAddr(opts.PublicIP)
		if err != nil {
			return fmt.Errorf("parse public IP: %w", err)
		}
		publicIPProto = pb.NewIP(ip)
	}

	// Register the machine in the cluster using its public key and endpoints from the token.
	endpoints := make([]*pb.IPPort, len(token.Endpoints))
	for i, addrPort := range token.Endpoints {
		endpoints[i] = pb.NewIPPort(addrPort)
	}
	addReq := &pb.AddMachineRequest{
		Name: opts.Name,
		Network: &pb.NetworkConfig{
			Endpoints: endpoints,
			PublicKey: token.PublicKey,
		},
		PublicIp: publicIPProto,
	}

	addResp, err := clusterClient.ClusterClient.AddMachine(ctx, addReq)
	if err != nil {
		return fmt.Errorf("add machine to cluster: %w", err)
	}

	// Get the current store DB version from the cluster to pass to the join request.
	var storeDBVersion int64
	inspectResp, err := clusterClient.MachineClient.InspectMachine(ctx, &emptypb.Empty{})
	if err != nil {
		if status.Convert(err).Code() != codes.Unimplemented {
			return fmt.Errorf("inspect current cluster machine: %w", err)
		}
	} else {
		storeDBVersion = inspectResp.Machines[0].StoreDbVersion
	}

	// Get the most up-to-date list of other machines in the cluster to include them in the join request.
	machines, err := clusterClient.ListMachines(ctx, nil)
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
		Machine:           addResp.Machine,
		OtherMachines:     otherMachines,
		MinStoreDbVersion: storeDBVersion,
	}
	if _, err = machineClient.MachineClient.JoinCluster(ctx, joinReq); err != nil {
		return fmt.Errorf("join cluster: %w", err)
	}

	fmt.Printf("Machine '%s' added to the cluster.\n", addResp.Machine.Name)

	// Wait for the cluster to be initialised on the machine to be able to deploy the Caddy service.
	if err = machineClient.WaitClusterReady(ctx, 5*time.Minute); err != nil {
		return fmt.Errorf("wait for machine to join the cluster: %w", err)
	}
	fmt.Println("Machine joined the cluster.")

	// Deploy Caddy service if it exists on other machines.
	if err = deployCaddyIfNeeded(ctx, clusterClient); err != nil {
		return err
	}

	fmt.Println()
	return caddy.UpdateDomainRecords(ctx, machineClient, progressOut())
}

func deployCaddyIfNeeded(ctx context.Context, clusterClient *client.Client) error {
	caddySvc, err := clusterClient.InspectService(ctx, client.CaddyServiceName)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			// Caddy service is not deployed.
			return nil
		}
		return fmt.Errorf("inspect caddy service: %w", err)
	}

	caddyImage := caddySvc.Containers[0].Container.Config.Image
	// Find the latest created container and use its image.
	var latestCreated time.Time
	for _, c := range caddySvc.Containers[1:] {
		created, err := time.Parse(time.RFC3339Nano, c.Container.Created)
		if err != nil {
			continue
		}
		if created.After(latestCreated) {
			latestCreated = created
			caddyImage = c.Container.Config.Image
		}
	}

	d, err := clusterClient.NewCaddyDeployment(caddyImage, "", api.Placement{})
	if err != nil {
		return fmt.Errorf("create caddy deployment: %w", err)
	}

	plan, err := d.Plan(ctx)
	if err != nil {
		return fmt.Errorf("plan caddy deployment: %w", err)
	}

	fmt.Println()
	if len(plan.Operations) == 0 {
		fmt.Printf("%s service is up to date.\n", client.CaddyServiceName)
		return nil
	}

	// Initialise a machine and container name resolver to properly format the plan output.
	resolver, err := clusterClient.ServiceOperationNameResolver(ctx, caddySvc)
	if err != nil {
		return fmt.Errorf("create machine and container name resolver for service operations: %w", err)
	}

	fmt.Println("caddy deployment plan:")
	fmt.Println(plan.Format(resolver))
	fmt.Println()

	err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
		if _, err = d.Run(ctx); err != nil {
			return fmt.Errorf("deploy caddy: %w", err)
		}
		return nil
	}, progressOut(), fmt.Sprintf("Deploying service %s (%s mode)", d.Spec.Name, d.Spec.Mode))

	return err
}

func provisionAndConnect(ctx context.Context, user, host string, port int, keyPath, version string) (*client.Client, error) {
	// If keyPath is actually key content, write it to a temp file.
	if strings.HasPrefix(keyPath, "-----BEGIN") {
		tmpFile, err := os.CreateTemp("", "ssh-key-*")
		if err != nil {
			return nil, fmt.Errorf("create temp key file: %w", err)
		}
		if _, err := tmpFile.WriteString(keyPath); err != nil {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			return nil, fmt.Errorf("write temp key file: %w", err)
		}
		tmpFile.Close()
		if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
			os.Remove(tmpFile.Name())
			return nil, fmt.Errorf("chmod temp key file: %w", err)
		}
		keyPath = tmpFile.Name()
		defer os.Remove(keyPath)
	}

	// Connect via SSH.
	sshClient, err := sshexec.Connect(user, host, port, keyPath)
	// If the SSH connection using SSH agent fails and no key path is provided, try to use the default SSH key.
	if err != nil && keyPath == "" {
		keyPath = cli.DefaultSSHKeyPath
		sshClient, err = sshexec.Connect(user, host, port, keyPath)
	}
	if err != nil {
		return nil, fmt.Errorf("SSH login to remote machine %s@%s:%d: %w", user, host, port, err)
	}

	// Provision the remote machine by installing the Uncloud daemon and dependencies over SSH.
	exec := sshexec.NewRemote(sshClient)
	if err = provisionMachine(ctx, exec, user, version); err != nil {
		return nil, fmt.Errorf("provision machine: %w", err)
	}

	// Create a machine API client over a new SSH connection (to pick up group membership changes).
	var machineClient *client.Client
	if user == rootUser {
		machineClient, err = client.New(ctx, connector.NewSSHConnectorFromClient(sshClient))
	} else {
		sshConfig := &connector.SSHConnectorConfig{
			User:    user,
			Host:    host,
			Port:    port,
			KeyPath: keyPath,
		}
		machineClient, err = client.New(ctx, connector.NewSSHConnector(sshConfig))
	}
	if err != nil {
		return nil, fmt.Errorf("connect to remote machine: %w", err)
	}

	return machineClient, nil
}

func provisionMachine(ctx context.Context, exec sshexec.Executor, user, version string) error {
	currentUser, err := exec.Run(ctx, "whoami")
	if err != nil {
		return fmt.Errorf("run whoami: %w", err)
	}

	if currentUser != rootUser {
		out, err := exec.Run(ctx, "sudo true")
		if err != nil {
			if strings.Contains(out, "password is required") {
				return fmt.Errorf(
					"user '%[1]s' requires a password for sudo, but Uncloud needs passwordless sudo or root access "+
						"to install and configure the uncloudd daemon on the remote machine.\n\n"+
						"Possible solutions:\n"+
						"1. Use root user or a user with passwordless sudo instead.\n"+
						"2. Configure passwordless sudo for the user '%[1]s' by running on the remote machine:\n"+
						"   echo '%[1]s ALL=(ALL) NOPASSWD:ALL' | sudo tee /etc/sudoers.d/%[1]s",
					currentUser)
			}
			return fmt.Errorf("sudo command failed for user '%s': %w. "+
				"Please ensure the user has sudo privileges or use root user instead", currentUser, err)
		}
	}

	cmd := installCmd(user, version)

	fmt.Println("Downloading Uncloud install script:", installScriptURL)

	cmd = sshexec.QuoteCommand("bash", "-c", "set -o pipefail; "+cmd)
	if err = exec.Stream(ctx, cmd, os.Stdout, os.Stderr); err != nil {
		return fmt.Errorf("download and run install script: %w", err)
	}
	return nil
}

func installCmd(user string, version string) string {
	sudoPrefix := ""
	var env []string

	if user != rootUser {
		sudoPrefix = "sudo"
		env = append(env, "UNCLOUD_GROUP_ADD_USER="+sshexec.Quote(user))
	}
	if version != "" {
		env = append(env, "UNCLOUD_VERSION="+sshexec.Quote(version))
	}

	envCmd := strings.Join(env, " ")
	return fmt.Sprintf("curl -fsSL %s | %s %s bash", sshexec.Quote(installScriptURL), sudoPrefix, envCmd)
}

func resetAndWaitMachine(ctx context.Context, machineClient pb.MachineClient) error {
	if _, err := machineClient.Reset(ctx, &pb.ResetRequest{}); err != nil {
		return fmt.Errorf("reset remote machine: %w. You can also manually run 'uncloud-uninstall' "+
			"on the remote machine to fully uninstall Uncloud from it", err)
	}

	fmt.Println("Resetting the remote machine...")
	if err := waitMachineReady(ctx, machineClient, 1*time.Minute); err != nil {
		return fmt.Errorf("wait for machine to be ready after reset: %w", err)
	}

	return nil
}

func waitMachineReady(ctx context.Context, machineClient pb.MachineClient, timeout time.Duration) error {
	boff := backoff.WithContext(backoff.NewExponentialBackOff(
		backoff.WithMaxInterval(1*time.Second),
		backoff.WithMaxElapsedTime(timeout),
	), ctx)

	inspect := func() error {
		_, err := machineClient.Inspect(ctx, &emptypb.Empty{})
		if err != nil {
			return fmt.Errorf("inspect machine: %w", err)
		}
		return nil
	}
	return backoff.Retry(inspect, boff)
}

func parseSSHDestination(dest string) (user, host string, port int, err error) {
	port = defaultSSHPort
	user = "root"

	// Handle user@host format.
	if idx := strings.LastIndex(dest, "@"); idx != -1 {
		user = dest[:idx]
		dest = dest[idx+1:]
	}

	// Handle host:port format.
	if idx := strings.LastIndex(dest, ":"); idx != -1 {
		host = dest[:idx]
		_, err = fmt.Sscanf(dest[idx+1:], "%d", &port)
		if err != nil {
			return "", "", 0, fmt.Errorf("invalid port in destination: %s", dest)
		}
	} else {
		host = dest
	}

	if host == "" {
		return "", "", 0, fmt.Errorf("empty host in destination")
	}

	return user, host, port, nil
}

func progressOut() *streams.Out {
	return streams.NewOut(os.Stdout)
}
