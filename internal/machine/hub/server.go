package hub

import (
	"time"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/internal/grpcversion"
	"github.com/siderolabs/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

const (
	// keepaliveTime is how long a tunnel can be idle before the machine pings Hub through it.
	keepaliveTime = 30 * time.Second
	// keepaliveTimeout is how long the machine waits for the reply before it closes the tunnel.
	keepaliveTimeout = 10 * time.Second
)

// allowedMethods are the Uncloud API methods that Hub can call through a tunnel. Hub has read-only access, so they
// must only read insensitive data. Hub must not see secrets or environment variables.
var allowedMethods = map[string]struct{}{
	pb.Cluster_ListMachines_FullMethodName:   {},
	pb.Machine_InspectMachine_FullMethodName: {},
}

// NewAPIServer returns the gRPC server that serves Hub requests from tunnels. Like the cluster API server, it routes
// requests to local or remote machines with the director, but only for the allowed methods. The allowlist must be
// enforced here because the machines behind the director trust each other's requests.
func NewAPIServer(director proxy.StreamDirector) *grpc.Server {
	return grpc.NewServer(
		grpc.ForceServerCodecV2(proxy.Codec()),
		// The unknown service handler serves all requests as streams so only stream interceptors are needed.
		// Don't register services on this server because their unary methods would skip the allowlist.
		grpc.ChainStreamInterceptor(allowlistInterceptor, grpcversion.ServerStreamInterceptor),
		grpc.UnknownServiceHandler(proxy.TransparentHandler(director)),
		// Ping Hub through the tunnel when it's idle. The pings travel inside the tunnel stream as data, so they check
		// the whole path to Hub, including proxies in front of it, and keep proxies from closing the tunnel as idle.
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    keepaliveTime,
			Timeout: keepaliveTimeout,
		}),
	)
}

func allowlistInterceptor(
	srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler,
) error {
	if _, ok := allowedMethods[info.FullMethod]; !ok {
		return status.Errorf(codes.PermissionDenied, "method %s is not allowed through a Hub tunnel", info.FullMethod)
	}
	return handler(srv, ss)
}
