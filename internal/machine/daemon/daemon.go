package daemon

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"log/slog"
	"net"
	"uncloud/internal/machine"
	"uncloud/internal/machine/api"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/network"
)

const (
	MachineAPIPort = 51000
)

func Run(ctx context.Context, dataDir string) error {
	cfg, err := machine.ParseConfig(machine.ConfigPath(dataDir))
	if err != nil {
		return fmt.Errorf("load machine config: %w", err)
	}

	wgnet, err := network.NewWireGuardNetwork()
	if err != nil {
		return fmt.Errorf("create WireGuard network: %w", err)
	}
	if err = wgnet.Configure(*cfg.Network); err != nil {
		return fmt.Errorf("configure WireGuard network: %w", err)
	}

	//ctx, cancel := context.WithCancel(context.Background())
	//go wgnet.WatchEndpoints(ctx, peerEndpointChangeNotifier)

	//addrs, err := network.ListRoutableAddresses()
	//if err != nil {
	//	return err
	//}
	//fmt.Println("Addresses:", addrs)

	addr := fmt.Sprintf("127.0.0.1:%d", MachineAPIPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen API port: %w", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterClusterServer(grpcServer, api.NewServer())

	// Use an errgroup to coordinate error handling and graceful shutdown of multiple daemon components.
	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		slog.Info("Starting API server.", "addr", addr)
		if sErr := grpcServer.Serve(listener); sErr != nil {
			return fmt.Errorf("API server failed: %w", sErr)
		}
		return nil
	})
	errGroup.Go(func() error {
		if err = wgnet.Run(ctx); err != nil {
			return fmt.Errorf("WireGuard network failed: %w", err)
		}
		return nil
	})
	// Shutdown goroutine.
	errGroup.Go(func() error {
		<-ctx.Done()
		slog.Info("Stopping API server.")
		// TODO: implement timeout for graceful shutdown.
		grpcServer.GracefulStop()
		slog.Info("API server stopped.")
		return nil
	})

	return errGroup.Wait()
}
