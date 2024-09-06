package machine

import (
	"fmt"
	"google.golang.org/grpc"
	"log/slog"
	"net"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/cluster"
)

type Config struct {
	// DataDir is the directory where the machine stores its persistent state.
	DataDir     string
	APIAddr     string
	APISockPath string
}

type Machine struct {
	config Config
	server *grpc.Server
}

func NewMachine(config *Config) (*Machine, error) {
	m := &Machine{
		config: *config,
		server: grpc.NewServer(),
	}

	clusterState := cluster.NewState(cluster.StatePath(config.DataDir))
	clusterServer := cluster.NewServer(clusterState)
	pb.RegisterClusterServer(m.server, clusterServer)

	return m, nil
}

func (m *Machine) Run() error {
	listener, err := net.Listen("tcp", m.config.APIAddr)
	if err != nil {
		return fmt.Errorf("listen API port: %w", err)
	}
	slog.Info("Starting API server.", "addr", m.config.APIAddr)
	if err = m.server.Serve(listener); err != nil {
		return fmt.Errorf("API server failed: %w", err)
	}
	return nil
}

func (m *Machine) Stop() {
	slog.Info("Stopping API server.")
	// TODO: implement timeout for graceful shutdown.
	m.server.GracefulStop()
	slog.Info("API server stopped.")
}
