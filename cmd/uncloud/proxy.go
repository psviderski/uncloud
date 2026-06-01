package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/proxy"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type proxyOptions struct {
	localPort  int
	remotePort int
	service    string
}

// NewProxyCommand creates a new command to proxy local ports to a service's port int the cluste.
func NewProxyCommand() *cobra.Command {
	opts := proxyOptions{}
	cmd := &cobra.Command{
		Use:   "proxy SERVICE [LOCAL_PORT:]REMOTE_PORT",
		Args:  cobra.ExactArgs(2),
		Short: "Proxy to service.",
		Long: `Proxy to a service on the remote port.

If no local port is provided a random port will be chosen.

The connection stays open for as long the command runs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			opts.service = args[0]

			parts := strings.Split(args[1], ":")
			switch len(parts) {
			case 1:
				remoteport, err := strconv.Atoi(parts[0])
				if err != nil {
					return fmt.Errorf("invalid remote port: '%s': %w", parts[0], err)
				}
				opts.remotePort = remoteport
			case 2:
				localport, err := strconv.Atoi(parts[0])
				if err != nil {
					return fmt.Errorf("invalid local port: '%s': %w", parts[0], err)
				}
				remoteport, err := strconv.Atoi(parts[1])
				if err != nil {
					return fmt.Errorf("invalid remote port: '%s': %w", parts[1], err)
				}
				opts.localPort = localport
				opts.remotePort = remoteport
			default:
				return fmt.Errorf("invalid port")
			}

			return runProxy(cmd.Context(), uncli, opts)
		},
	}

	return cmd
}

func runProxy(ctx context.Context, uncli *cli.CLI, opts proxyOptions) error {
	if opts.localPort < 0 || opts.localPort > 65535 {
		return fmt.Errorf("invalid local port %d: must be between 0 and 65535", opts.localPort)
	}
	if opts.remotePort < 1 || opts.remotePort > 65535 {
		return fmt.Errorf("invalid remote port %d: must be between 1 and 65535", opts.remotePort)
	}

	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	svc, err := clusterClient.InspectService(ctx, opts.service)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			return fmt.Errorf("service '%s' not found in the cluster", opts.service)
		}
		return fmt.Errorf("inspect service '%s': %w", opts.service, err)
	}

	dialer, err := clusterClient.Connector.Dialer()
	if err != nil {
		return fmt.Errorf("get proxy dialer: %w", err)
	}

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(opts.localPort)))
	if err != nil {
		return fmt.Errorf("listen on port on 127.0.0.1: %w", err)
	}

	ip := svc.Containers[0].Container.UncloudNetworkIP()

	// There is no precheck if we can connect, as this always succeeds, only the proxy connects with the
	// endpoint and shuffles the data, *it* will actually experience errors.
	remoteAddr := net.JoinHostPort(ip.String(), strconv.Itoa(opts.remotePort))
	fmt.Printf("Connecting to '%s'\n", remoteAddr)

	ctx, cancel := context.WithCancel(ctx)
	p := &proxy.Proxy{
		Listener:    listener,
		RemoteAddr:  remoteAddr,
		DialContext: dialer.DialContext,
		OnError: func(err error) {
			fmt.Printf("Failed to proxy to '%s': %v\n", remoteAddr, err)
			cancel()
		},
	}

	go p.Run(ctx)

	fmt.Printf("%s → %s:%d\n", p.Listener.Addr().String(), opts.service, opts.remotePort)

	<-ctx.Done()
	fmt.Println("Closed")

	return nil
}
