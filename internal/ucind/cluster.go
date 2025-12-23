package ucind

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/psviderski/uncloud/internal/machine"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/cluster"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	ClusterNameLabel = "ucind.cluster.name"
	ManagedLabel     = "ucind.managed"
)

type Cluster struct {
	Name     string
	Machines []Machine
}

type CreateClusterOptions struct {
	Machines int
	// Ports to forward from the cluster machines to the host.
	PortMap nat.PortMap
}

func (c *Cluster) PopulateMachineIDs(ctx context.Context) error {
	for i := range c.Machines {
		if c.Machines[i].ID != "" {
			continue
		}

		cli, err := c.Machines[i].Connect(ctx)
		if err != nil {
			return fmt.Errorf("connect to machine '%s': %w", c.Machines[i].Name, err)
		}
		//goland:noinspection GoDeferInLoop
		defer cli.Close()

		m, err := cli.Inspect(ctx, &emptypb.Empty{})
		if err != nil {
			return fmt.Errorf("inspect machine '%s': %w", c.Machines[i].Name, err)
		}
		c.Machines[i].ID = m.Id
	}

	return nil
}

func (p *Provisioner) CreateCluster(ctx context.Context, name string, opts CreateClusterOptions) (Cluster, error) {
	var c Cluster

	_, err := p.InspectCluster(ctx, name)
	if err == nil {
		return c, fmt.Errorf("cluster with name '%s' already exists", name)
	}
	if !errors.Is(err, ErrNotFound) {
		return c, fmt.Errorf("inspect cluster '%s': %w", name, err)
	}

	netOpts := network.CreateOptions{
		Labels: map[string]string{
			ClusterNameLabel: name,
			ManagedLabel:     "",
		},
	}
	// Create a Docker network with the same name as the cluster name.
	if _, err = p.dockerCli.NetworkCreate(ctx, name, netOpts); err != nil {
		return c, fmt.Errorf("create Docker network '%s': %w", name, err)
	}
	c.Name = name

	// Create machines (containers) in the created cluster network.
	for i := 1; i < opts.Machines+1; i++ {
		mopts := CreateMachineOptions{
			Name:    fmt.Sprintf("machine-%d", i),
			PortMap: opts.PortMap,
		}
		m, err := p.CreateMachine(ctx, name, mopts)
		if err != nil {
			return c, fmt.Errorf("create machine '%s': %w", mopts.Name, err)
		}

		c.Machines = append(c.Machines, m)
	}

	if len(c.Machines) == 0 {
		return c, nil
	}

	if err = p.initCluster(ctx, c.Machines); err != nil {
		return c, err
	}
	if err = c.PopulateMachineIDs(ctx); err != nil {
		return c, fmt.Errorf("populate machine IDs: %w", err)
	}

	if p.configUpdater != nil {
		if err = p.configUpdater.AddCluster(c); err != nil {
			return c, fmt.Errorf("add cluster to Uncloud config: %w", err)
		}
		fmt.Printf("Cluster '%s' added to Uncloud config as the current context.\n", name)
	}

	return c, nil
}

func (p *Provisioner) initCluster(ctx context.Context, machines []Machine) error {
	// Init a new cluster on the first machine.
	initMachine := machines[0]
	initClient, err := initMachine.Connect(ctx)
	if err != nil {
		return fmt.Errorf("create machine client over TCP '%s': %w", initMachine.APIAddress, err)
	}
	defer initClient.Close()

	if err := initClient.WaitMachineReady(ctx, 30*time.Second); err != nil {
		return fmt.Errorf("wait for machine %q to be ready: %w", initMachine.Name, err)
	}

	req := &pb.InitClusterRequest{
		MachineName: initMachine.Name,
		Network:     pb.NewIPPrefix(cluster.DefaultNetwork),
	}
	initResp, err := initClient.InitCluster(ctx, req)
	if err != nil {
		return fmt.Errorf("init cluster: %w", err)
	}

	fmt.Printf("Cluster %q initialised with machine %q\n", initMachine.ClusterName, initResp.Machine.Name)
	fmt.Printf("Waiting for cluster to be ready...")
	if err = initClient.WaitClusterReady(ctx, 30*time.Second); err != nil {
		return fmt.Errorf("wait for cluster to be ready: %w", err)
	}
	fmt.Println(" done.")

	// Get the current store DB version from the init machine to pass to the join requests.
	inspectResp, err := initClient.MachineClient.InspectMachine(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("inspect init machine: %w", err)
	}

	// Join the rest of the machines to the cluster.
	for _, m := range machines[1:] {
		cli, err := m.Connect(ctx)
		if err != nil {
			return fmt.Errorf("create machine client over TCP '%s': %w", m.APIAddress, err)
		}
		//goland:noinspection GoDeferInLoop
		defer cli.Close()

		if err := cli.WaitMachineReady(ctx, 30*time.Second); err != nil {
			return fmt.Errorf("wait for machine %q to be ready: %w", m.Name, err)
		}

		tokenResp, err := cli.Token(ctx, &emptypb.Empty{})
		if err != nil {
			return fmt.Errorf("get machine token: %w", err)
		}
		token, err := machine.ParseToken(tokenResp.Token)
		if err != nil {
			return fmt.Errorf("parse machine token: %w", err)
		}

		// Register the machine in the cluster using its public key and endpoints from the token.
		endpoints := make([]*pb.IPPort, len(token.Endpoints))
		for i, addrPort := range token.Endpoints {
			endpoints[i] = pb.NewIPPort(addrPort)
		}
		addReq := &pb.AddMachineRequest{
			Name: m.Name,
			Network: &pb.NetworkConfig{
				Endpoints: endpoints,
				PublicKey: token.PublicKey,
			},
		}
		addResp, err := initClient.AddMachine(ctx, addReq)
		if err != nil {
			return fmt.Errorf("add machine to cluster: %w", err)
		}

		// Configure the machine to join the cluster.
		joinReq := &pb.JoinClusterRequest{
			Machine:           addResp.Machine,
			OtherMachines:     []*pb.MachineInfo{initResp.Machine},
			MinStoreDbVersion: inspectResp.Machines[0].StoreDbVersion,
		}
		if _, err = cli.JoinCluster(ctx, joinReq); err != nil {
			return fmt.Errorf("join cluster: %w", err)
		}

		fmt.Printf("Machine %q added to cluster\n", addResp.Machine.Name)
	}

	return nil
}

func (p *Provisioner) InspectCluster(ctx context.Context, name string) (Cluster, error) {
	var c Cluster

	// Docker network name is the same as the cluster name.
	net, err := p.dockerCli.NetworkInspect(ctx, name, network.InspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return c, ErrNotFound
		}
		return c, fmt.Errorf("inspect Docker network '%s': %w", name, err)
	}

	if _, ok := net.Labels[ManagedLabel]; !ok {
		// The network with the cluster name exists, but it's not managed by ucind.
		return c, ErrNotFound
	}

	c.Name = name
	// Include all containers (machines) with the cluster name label.
	opts := container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", ClusterNameLabel+"="+name),
			filters.Arg("label", ManagedLabel),
		),
	}
	containers, err := p.dockerCli.ContainerList(ctx, opts)
	if err != nil {
		return c, fmt.Errorf("list Docker containers with cluster name '%s': %w", name, err)
	}
	for _, ctr := range containers {
		m := Machine{
			ClusterName:   name,
			ContainerName: ctr.Names[0],
			Name:          ctr.Labels[MachineNameLabel],
		}

		for _, port := range ctr.Ports {
			if port.PrivatePort == UncloudAPIPort {
				if ip, err := netip.ParseAddr(port.IP); err == nil {
					m.APIAddress = netip.AddrPortFrom(ip, port.PublicPort)
					break
				}
			}
		}
		if !m.APIAddress.IsValid() {
			return c, fmt.Errorf("API binding not found for container '%s'", m.ContainerName)
		}

		c.Machines = append(c.Machines, m)
	}

	if err = c.PopulateMachineIDs(ctx); err != nil {
		return c, fmt.Errorf("populate machine IDs: %w", err)
	}

	return c, nil
}

// WaitClusterReady waits for all machines in the cluster to be ready and UP.
func (p *Provisioner) WaitClusterReady(ctx context.Context, c Cluster, timeout time.Duration) error {
	firstMachine := c.Machines[0]
	cli, err := firstMachine.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect to machine over TCP '%s': %w", firstMachine.APIAddress, err)
	}
	defer cli.Close()

	if err = cli.WaitClusterReady(ctx, timeout); err != nil {
		return fmt.Errorf("wait for cluster to be ready: %w", err)
	}

	boff := backoff.WithContext(backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(100*time.Millisecond),
		backoff.WithMaxInterval(1*time.Second),
		backoff.WithMaxElapsedTime(timeout),
	), ctx)

	checkMachinesUp := func() error {
		machines, err := cli.ListMachines(ctx, nil)
		if err != nil {
			return fmt.Errorf("list machines: %w", err)
		}

		if len(machines) != len(c.Machines) {
			return fmt.Errorf("expected %d machines, got %d", len(c.Machines), len(machines))
		}

		for _, m := range machines {
			if m.State != pb.MachineMember_UP {
				return fmt.Errorf("machine '%s' state is not UP: %s", m.Machine.Name, m.State)
			}
		}
		return nil
	}
	if err = backoff.Retry(checkMachinesUp, boff); err != nil {
		return err
	}

	// Wait for each machine to sync the cluster store and be ready to serve cluster requests.
	for _, m := range c.Machines[1:] {
		mcli, err := m.Connect(ctx)
		if err != nil {
			return fmt.Errorf("connect to machine over TCP '%s': %w", m.APIAddress, err)
		}
		//goland:noinspection GoDeferInLoop
		defer mcli.Close()

		if err = mcli.WaitClusterReady(ctx, timeout); err != nil {
			return fmt.Errorf("wait for cluster to be ready on machine '%s': %w", m.Name, err)
		}
	}

	return nil
}

func (p *Provisioner) RemoveCluster(ctx context.Context, name string) error {
	if _, err := p.InspectCluster(ctx, name); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}

	// Remove all containers (machines) with the cluster name label.
	opts := container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", ClusterNameLabel+"="+name),
			filters.Arg("label", ManagedLabel),
		),
	}
	containers, err := p.dockerCli.ContainerList(ctx, opts)
	if err != nil {
		return fmt.Errorf("list Docker containers with cluster name '%s': %w", name, err)
	}
	for _, c := range containers {
		removeOpts := container.RemoveOptions{
			// Remove anonymous volumes attached to the container (typically /var/lib/docker).
			RemoveVolumes: true,
			Force:         true,
		}
		if err = p.dockerCli.ContainerRemove(ctx, c.ID, removeOpts); err != nil {
			return fmt.Errorf("remove Docker container '%s': %w", c.ID, err)
		}
	}

	if err = p.dockerCli.NetworkRemove(ctx, name); err != nil {
		return fmt.Errorf("remove Docker network '%s': %w", name, err)
	}

	if p.configUpdater != nil {
		if err = p.configUpdater.RemoveCluster(name); err != nil {
			return fmt.Errorf("remove cluster from Uncloud config: %w", err)
		}
		fmt.Printf("Cluster '%s' removed from Uncloud config.\n", name)
	}

	return nil
}
