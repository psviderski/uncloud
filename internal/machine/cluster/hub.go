package cluster

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/internal/machine/hub"
	"github.com/psviderski/uncloud/internal/machine/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// connectHubTimeout is how long to wait for Hub to connect the cluster.
const connectHubTimeout = 30 * time.Second

// ConnectHub exchanges the connection token for the ID of the cluster in Hub and sends Hub the WireGuard public keys
// of all machines in the cluster. It then stores the cluster connection in the cluster store so that every machine
// opens a tunnel to Hub.
func (c *Cluster) ConnectHub(ctx context.Context, req *pb.ConnectHubRequest) (*pb.HubConnection, error) {
	if err := c.checkReady(); err != nil {
		return nil, err
	}
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "connection token not set")
	}

	if conn, err := c.store.Hub.GetConnection(ctx); err == nil {
		return nil, status.Errorf(codes.AlreadyExists, "cluster is already connected to Hub at %s (cluster ID: %s)",
			conn.Endpoint, conn.ClusterID)
	} else if !errors.Is(err, store.ErrKeyNotFound) {
		return nil, status.Errorf(codes.Internal, "get Hub connection from store: %v", err)
	}

	hubClient, err := hub.NewClient(req.Endpoint)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer hubClient.Close()

	machines, err := c.store.ListMachines(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list machines: %v", err)
	}

	hubCtx, cancel := context.WithTimeout(ctx, connectHubTimeout)
	defer cancel()
	resp, err := hubClient.ConnectCluster(hubCtx, &pb.ConnectClusterRequest{
		Token:    req.Token,
		Machines: machines,
	})
	if err != nil {
		// Clients can retry Unavailable errors, which is safe because ConnectCluster is idempotent for the same token
		// and machines.
		return nil, err
	}
	if resp.ClusterId == "" {
		return nil, status.Error(codes.Internal, "Hub returned an empty cluster ID")
	}

	if err = c.store.Hub.PutConnection(ctx, store.HubConnection{
		Endpoint:  req.Endpoint,
		ClusterID: resp.ClusterId,
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "store Hub connection: %v", err)
	}
	slog.Info("Cluster connected to Hub.", "endpoint", req.Endpoint, "cluster_id", resp.ClusterId)

	return c.hubConnection(ctx)
}

// DisconnectHub removes the cluster connection so that every machine closes its tunnel to Hub.
func (c *Cluster) DisconnectHub(ctx context.Context, _ *emptypb.Empty) (*pb.HubConnection, error) {
	if err := c.checkReady(); err != nil {
		return nil, err
	}

	conn, err := c.hubConnection(ctx)
	if err != nil {
		return nil, err
	}
	if err = c.store.Hub.DeleteConnection(ctx); err != nil {
		return nil, status.Errorf(codes.Internal, "delete Hub connection from store: %v", err)
	}
	slog.Info("Cluster disconnected from Hub.", "endpoint", conn.Endpoint, "cluster_id", conn.ClusterId)

	return conn, nil
}

func (c *Cluster) InspectHubConnection(ctx context.Context, _ *emptypb.Empty) (*pb.HubConnection, error) {
	if err := c.checkReady(); err != nil {
		return nil, err
	}
	return c.hubConnection(ctx)
}

func (c *Cluster) hubConnection(ctx context.Context) (*pb.HubConnection, error) {
	conn, err := c.store.Hub.GetConnection(ctx)
	if err != nil {
		if errors.Is(err, store.ErrKeyNotFound) {
			return nil, status.Error(codes.NotFound, "cluster is not connected to Hub")
		}
		return nil, status.Errorf(codes.Internal, "get Hub connection from store: %v", err)
	}

	return &pb.HubConnection{
		Endpoint:  conn.Endpoint,
		ClusterId: conn.ClusterID,
		CreatedAt: timestamppb.New(conn.CreatedAt),
	}, nil
}
