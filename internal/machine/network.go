package machine

import (
	"context"
	"fmt"
	dnetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/libnetwork/iptables"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"time"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/network"
)

const (
	APIPort           = 51000
	DockerNetworkName = "uncloud"
	DockerUserChain   = "DOCKER-USER"
)

type networkController struct {
	state         *State
	wgnet         *network.WireGuardNetwork
	server        *grpc.Server
	newMachinesCh <-chan *pb.MachineInfo
	// TODO: DNS server/resolver listening on the machine IP, e.g. 10.210.0.1:53. It can't listen on 127.0.X.X
	//  like resolved does because it needs to be reachable from both the host and the containers.
}

func newNetworkController(state *State, server *grpc.Server, newMachCh <-chan *pb.MachineInfo) (
	*networkController, error,
) {
	slog.Info("Starting WireGuard network.")
	wgnet, err := network.NewWireGuardNetwork()
	if err != nil {
		return nil, fmt.Errorf("create WireGuard network: %w", err)
	}

	return &networkController{
		state:         state,
		wgnet:         wgnet,
		server:        server,
		newMachinesCh: newMachCh,
	}, nil
}

func (nc *networkController) Run(ctx context.Context) error {
	if err := nc.wgnet.Configure(*nc.state.Network); err != nil {
		return fmt.Errorf("configure WireGuard network: %w", err)
	}
	slog.Info("WireGuard network configured.")

	errGroup, ctx := errgroup.WithContext(ctx)

	// Start the network API server. Assume the management IP can't be changed when the network is running.
	apiAddr := net.JoinHostPort(nc.state.Network.ManagementIP.String(), strconv.Itoa(APIPort))
	listener, err := net.Listen("tcp", apiAddr)
	if err != nil {
		return fmt.Errorf("listen API port: %w", err)
	}
	errGroup.Go(
		func() error {
			slog.Info("Starting network API server.", "addr", apiAddr)
			if err := nc.server.Serve(listener); err != nil {
				return fmt.Errorf("network API server failed: %w", err)
			}
			return nil
		},
	)

	// Setup Docker network and iptables rules in a goroutine because it may block until the Docker daemon is ready.
	errGroup.Go(
		func() error {
			if err := nc.setupDockerNetwork(ctx); err != nil {
				return fmt.Errorf("setup Docker network: %w", err)
			}
			slog.Info("Docker network configured.")
			return nil
		},
	)

	// Handle new machines added to the cluster. Handling new machines and endpoint changes should be done
	// in separate goroutines to avoid a deadlock when reconfiguring the network.
	errGroup.Go(
		func() error {
			if err := nc.handleNewMachines(ctx); err != nil {
				return fmt.Errorf("handle new machines: %w", err)
			}
			return nil
		},
	)

	// TODO: run another goroutine to watch WG network endpoint changes and update the state accordingly.
	//  Network updates in the state should not occur outside of this controller.

	errGroup.Go(
		func() error {
			if err = nc.wgnet.Run(ctx); err != nil {
				return fmt.Errorf("WireGuard network failed: %w", err)
			}
			return nil
		},
	)

	// Wait for the context to be done and stop the network API server.
	errGroup.Go(
		func() error {
			<-ctx.Done()
			slog.Info("Stopping network API server.")
			// TODO: implement timeout for graceful shutdown.
			nc.server.GracefulStop()
			slog.Info("Network API server stopped.")
			return nil
		},
	)

	return errGroup.Wait()
}

// setupDockerNetwork creates the Docker bridge network DockerNetworkName with the machine subnet and configures
// iptables to allow WireGuard network to access containers.
func (nc *networkController) setupDockerNetwork(ctx context.Context) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("init Docker client: %w", err)
	}
	defer cli.Close()

	// Wait for the Docker daemon to start and be ready by sending a ping request in a loop.
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	ready, waitingLogged := false, false
	for !ready {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_, err = cli.Ping(ctx)
			if err == nil {
				ready = true
				break
			}
			if !client.IsErrConnectionFailed(err) {
				return fmt.Errorf("connect to Docker daemon: %w", err)
			}
			if !waitingLogged {
				slog.Info("Waiting for Docker daemon to start and be ready to setup Docker network.")
				waitingLogged = true
			}
		}
	}

	// Ensure the Docker network 'uncloud' is created with the correct subnet.
	needsCreation := false
	nw, err := cli.NetworkInspect(ctx, DockerNetworkName, dnetwork.InspectOptions{})
	if err != nil {
		if !client.IsErrNotFound(err) {
			return fmt.Errorf("inspect Docker network %q: %w", DockerNetworkName, err)
		}
		needsCreation = true
	} else if nw.IPAM.Config[0].Subnet != nc.state.Network.Subnet.String() {
		// Remove the Docker network if the subnet is different.
		// It could be a leftover from a previous incomplete cleanup.
		slog.Info(
			"Removing Docker network with old subnet.", "name", DockerNetworkName, "subnet", nw.IPAM.Config[0].Subnet,
		)
		if err = cli.NetworkRemove(ctx, DockerNetworkName); err != nil {
			// It can still fail if the network is in use by a container. Leave it to the user to resolve the issue.
			return fmt.Errorf("remove Docker network %q: %w", DockerNetworkName, err)
		}
		needsCreation = true
	}

	if needsCreation {
		if _, err = cli.NetworkCreate(
			ctx, DockerNetworkName, dnetwork.CreateOptions{
				Driver: "bridge",
				Scope:  "local",
				IPAM: &dnetwork.IPAM{
					Config: []dnetwork.IPAMConfig{
						{
							Subnet: nc.state.Network.Subnet.String(),
						},
					},
				},
			},
		); err != nil {
			return fmt.Errorf("create Docker network %q: %w", DockerNetworkName, err)
		}
		slog.Info("Docker network created.", "name", DockerNetworkName, "subnet", nc.state.Network.Subnet.String())

		if nw, err = cli.NetworkInspect(ctx, DockerNetworkName, dnetwork.InspectOptions{}); err != nil {
			return fmt.Errorf("inspect Docker network %q: %w", DockerNetworkName, err)
		}
	}

	// Configure iptables to allow WireGuard network to access containers. The Docker daemon should have already
	// created the DOCKER-USER chain at this point.
	// TODO: check if this works when firewalld used instead of raw iptables. The Docker daemon has a different
	//  code path for firewalld.

	// Bridge name doesn't seem to be documented but this is the source code where it is generated:
	// https://github.com/moby/moby/blob/v27.2.1/libnetwork/drivers/bridge/bridge_linux.go#L664
	bridgeName := "br-" + nw.ID[:12]
	ipt := iptables.GetIptable(iptables.IPv4)
	rule := []string{"--in-interface", network.WireGuardInterfaceName, "--out-interface", bridgeName, "-j", "ACCEPT"}
	if err = ipt.ProgramRule(iptables.Filter, DockerUserChain, iptables.Insert, rule); err != nil {
		return fmt.Errorf("insert iptables rule: %w", err)
	}

	return nil
}

func (nc *networkController) handleNewMachines(ctx context.Context) error {
	for {
		select {
		case minfo := <-nc.newMachinesCh:
			slog.Info("Handling new machine added to the cluster.", "name", minfo.Name)

			// Skip the current machine.
			nc.state.mu.RLock()
			currentMachID := nc.state.ID
			nc.state.mu.RUnlock()
			if minfo.Id == currentMachID {
				continue
			}

			if err := minfo.Network.Validate(); err != nil {
				slog.Error("Invalid machine network configuration.", "err", err)
				continue
			}
			// Ignore errors as they are already validated.
			subnet, _ := minfo.Network.Subnet.ToPrefix()
			manageIP, _ := minfo.Network.ManagementIp.ToAddr()
			endpoints := make([]netip.AddrPort, len(minfo.Network.Endpoints))
			for i, ep := range minfo.Network.Endpoints {
				addrPort, _ := ep.ToAddrPort()
				endpoints[i] = addrPort
			}

			peer := network.PeerConfig{
				Subnet:       &subnet,
				ManagementIP: manageIP,
				AllEndpoints: endpoints,
				PublicKey:    minfo.Network.PublicKey,
			}
			if len(endpoints) > 0 {
				peer.Endpoint = &endpoints[0]
			}

			nc.state.mu.Lock()
			// TODO: deduplicate peers by public key, maybe implement addNetworkPeer method in the state.
			nc.state.Network.Peers = append(nc.state.Network.Peers, peer)
			err := nc.state.Save()
			nc.state.mu.Unlock()
			if err != nil {
				return fmt.Errorf("save machine state: %w", err)
			}

			if err := nc.wgnet.Configure(*nc.state.Network); err != nil {
				return fmt.Errorf("configure network with new peer: %w", err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// TODO: method to shutdown network when leaving a cluster. Regular context cancellation shouldn't bring it down.
