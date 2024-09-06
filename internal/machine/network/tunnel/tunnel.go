package tunnel

import (
	"context"
	"fmt"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"net"
	"net/netip"
	"time"
	"uncloud/internal/secret"
)

const (
	DefaultEndpointPort = 51820
	// DefaultKeepaliveInterval is sensible interval that works with a wide variety of firewalls.
	DefaultKeepaliveInterval = 25 * time.Second
)

type Tunnel struct {
	dev *device.Device
	net *netstack.Net
}

type Config struct {
	LocalAddress    netip.Addr
	LocalPrivateKey secret.Secret
	Endpoint        netip.AddrPort
	RemotePublicKey secret.Secret
	RemoteNetwork   netip.Prefix
	DNS             *netip.Addr
	MTU             int
	KeepAlive       time.Duration
}

func Connect(config *Config) (*Tunnel, error) {
	var dns netip.Addr
	if config.DNS != nil {
		dns = *config.DNS
	} else {
		dns = netip.MustParseAddr("1.1.1.1")
	}
	mtu := config.MTU
	if mtu == 0 {
		mtu = device.DefaultMTU
	}
	keepAlive := config.KeepAlive
	if keepAlive == 0 {
		keepAlive = DefaultKeepaliveInterval
	}

	tun, tnet, err := netstack.CreateNetTUN([]netip.Addr{config.LocalAddress}, []netip.Addr{dns}, mtu)
	if err != nil {
		return nil, fmt.Errorf("create WireGuard TUN device: %w", err)
	}

	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelError, "WireGuard tunnel: "))
	conf := fmt.Sprintf(
		"private_key=%s\n"+
			"public_key=%s\n"+
			"endpoint=%s\n"+
			"allowed_ip=%s\n"+
			"persistent_keepalive_interval=%d\n",
		config.LocalPrivateKey.String(),
		config.RemotePublicKey.String(),
		config.Endpoint.String(),
		config.RemoteNetwork.String(),
		int(keepAlive.Seconds()),
	)
	err = dev.IpcSet(conf)
	if err != nil {
		return nil, fmt.Errorf("configure WireGuard device: %w", err)
	}

	err = dev.Up()
	if err != nil {
		return nil, fmt.Errorf("enable WireGuard device: %w", err)
	}

	return &Tunnel{
		dev: dev,
		net: tnet,
	}, nil
}

func (t *Tunnel) Close() {
	if t.dev != nil {
		t.dev.Close()
	}
	t.dev, t.net = nil, nil
}

func (t *Tunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return t.net.DialContext(ctx, network, address)
}
