package connector

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"

	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/psviderski/uncloud/internal/machine/constants"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/psviderski/uncloud/internal/machine/network/tunnel"
	"github.com/psviderski/uncloud/pkg/client"
	"golang.org/x/net/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// WireGuardConnector establishes a connection to the cluster API through a WireGuard tunnel
// to one of the cluster machines.
type WireGuardConnector struct {
	user     *client.User
	machines []config.MachineConnection
	tun      *tunnel.Tunnel
}

func NewWireGuardConnector(user *client.User, machines []config.MachineConnection) *WireGuardConnector {
	return &WireGuardConnector{
		user:     user,
		machines: machines,
	}
}

// TODO: handle context cancelation.
func (c *WireGuardConnector) Connect(ctx context.Context) (*grpc.ClientConn, error) {
	if len(c.machines) == 0 {
		return nil, fmt.Errorf("no machines to connect to")
	}
	// TODO: iterate over machines and try to connect to each one until successful.
	//  For now, try to connect to only the first machine.
	machine := c.machines[0]
	endpointIPs, err := net.LookupIP(machine.Host)
	if err != nil {
		return nil, fmt.Errorf("resolve IP for %q: %w", machine.Host, err)
	}
	endpointAddr, err := netip.ParseAddr(endpointIPs[0].String())
	if err != nil {
		return nil, fmt.Errorf("parse IP address %q: %w", endpointIPs[0].String(), err)
	}
	endpoint := netip.AddrPortFrom(endpointAddr, tunnel.DefaultEndpointPort)
	machineManagementIP := network.ManagementIP(machine.PublicKey)
	machineAPIAddr := net.JoinHostPort(machineManagementIP.String(), strconv.Itoa(constants.MachineAPIPort))

	tunCfg := &tunnel.Config{
		LocalAddress:    c.user.ManagementIP(),
		LocalPrivateKey: c.user.PrivateKey(),
		RemotePublicKey: machine.PublicKey,
		RemoteNetwork:   netip.PrefixFrom(machineManagementIP, 128),
		Endpoint:        endpoint,
	}
	if c.tun, err = tunnel.Connect(tunCfg); err != nil {
		return nil, fmt.Errorf("establish WireGuard tunnel to %q: %w", endpoint, err)
	}

	conn, err := grpc.NewClient(
		machineAPIAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(defaultServiceConfig),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return c.tun.DialContext(ctx, "tcp", addr)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to machine API through WireGuard tunnel: %w", err)
	}
	return conn, nil
}

func (c *WireGuardConnector) Dialer() (proxy.ContextDialer, error) {
	return nil, fmt.Errorf("proxy connections not implemented for WireGuard connector")
}

func (c *WireGuardConnector) Close() error {
	if c.tun != nil {
		c.tun.Close()
		c.tun = nil
	}
	return nil
}
