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
		Use:   "proxy [LOCAL_PORT:]CONTAINER:REMMOTE_PORT",
		Args:  cobra.ExactArgs(1),
		Short: "Proxy to service.",
		Long: `Proxy to a container on the remote port.

If no local port is provided a random port will be chosen.

The connection stays open for as long the command runs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			parts := strings.Split(args[0], ":")
			switch len(parts) {
			case 2:
				remoteport, err := strconv.Atoi(parts[1])
				if err != nil {
					return fmt.Errorf("invalid remote port: '%s': %w", parts[1], err)
				}
				opts.service = parts[0]
				opts.remotePort = remoteport
			case 3:
				localport, err := strconv.Atoi(parts[0])
				if err != nil {
					return fmt.Errorf("invalid local port: '%s': %w", parts[0], err)
				}
				remoteport, err := strconv.Atoi(parts[2])
				if err != nil {
					return fmt.Errorf("invalid remove port: '%s': %w", parts[2], err)
				}
				opts.service = parts[1]
				opts.localPort = localport
				opts.remotePort = remoteport
			default:
				return fmt.Errorf("invalid container or port")

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

	ip := svc.Containers[0].Container.UncloudNetworkIP()

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(opts.localPort)))
	if err != nil {
		return fmt.Errorf("listen on port on 127.0.0.1: %w", err)
	}

	dialer, err := clusterClient.Connector.Dialer()
	if err != nil {
		return fmt.Errorf("get proxy dialer: %w", err)
	}

	p := &proxy.Proxy{
		Listener:    listener,
		RemoteAddr:  net.JoinHostPort(ip.String(), strconv.Itoa(opts.remotePort)),
		DialContext: dialer.DialContext,
	}

	p.Run(ctx)

	return nil
}
