package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/internal/proxy"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type proxyOptions struct {
	localPort  int
	remotePort int
	service    string
}

// NewProxyCommand creates a new command to proxy a local port to a service's port in the cluster.
func NewProxyCommand() *cobra.Command {
	opts := proxyOptions{}
	cmd := &cobra.Command{
		Use:   "proxy SERVICE [LOCAL_PORT:]REMOTE_PORT",
		Args:  cobra.ExactArgs(2),
		Short: "Proxy a service port to a local port.",
		Long: `Proxy a service port in the cluster to a local port on this machine.

If the service runs multiple containers, the command connects to the first running and healthy one.
If you don't provide a local port, the command picks a random one.

The connection stays open for as long as the command runs.`,
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
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return completion.Services(cmd.Context(), uncli, args, toComplete)
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

	// Pick the first running and healthy container to proxy to.
	var ctr *api.MachineServiceContainer
	for i := range svc.Containers {
		if svc.Containers[i].Container.Healthy() {
			ctr = &svc.Containers[i]
			break
		}
	}
	if ctr == nil {
		return fmt.Errorf("no running healthy container found for service '%s'", opts.service)
	}

	containerID := ctr.Container.ShortID()
	ip := ctr.Container.UncloudNetworkIP()
	if !ip.IsValid() {
		return fmt.Errorf("container '%s' is not connected to the uncloud Docker network (could be host network)",
			containerID)
	}

	dialer, err := clusterClient.Dialer()
	if err != nil {
		return fmt.Errorf("get proxy dialer: %w", err)
	}

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(opts.localPort)))
	if err != nil {
		return fmt.Errorf("listen on 127.0.0.1:%d: %w", opts.localPort, err)
	}

	// There is no precheck if we can connect, as this always succeeds, only the proxy connects with the
	// endpoint and shuffles the data, *it* will actually experience errors.
	remoteAddr := net.JoinHostPort(ip.String(), strconv.Itoa(opts.remotePort))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	p := &proxy.Proxy{
		Listener:    listener,
		RemoteAddr:  remoteAddr,
		DialContext: dialer.DialContext,
		OnError: func(err error) {
			fmt.Printf("Failed to proxy to '%s': %v\n", remoteAddr, err)
			cancel()
		},
	}

	// Run the proxy in the background and signal when it has fully shut down.
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	// Prefix the local address with the scheme for common HTTP ports so it becomes control-clickable in most
	// terminals. We assume plain HTTP since TLS is typically terminated by Caddy in front of the service.
	fmt.Printf("%s%s → %s (%s%s%s)\n", schemeForPort(opts.remotePort), p.Listener.Addr().String(),
		remoteAddr, opts.service, tui.Faint.Render("/"), containerID)

	<-ctx.Done()
	// Wait for the proxy to drain in-flight connections and shut down gracefully.
	<-done

	return nil
}

// schemeForPort returns the "http://" URL scheme prefix for the ports most likely to serve plain HTTP,
// or an empty string otherwise.
func schemeForPort(port int) string {
	switch port {
	case 80, 3000, 8000, 8080, 8081, 8888, 9090:
		return "http://"
	default:
		return ""
	}
}
