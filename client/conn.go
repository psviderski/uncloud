// Package client provides clients for the Uncloud API.
// TODO: migrate pkg/client to this package (breaks all open PRs that change pkg/client).
package client

import (
	"slices"

	"github.com/psviderski/uncloud/internal/grpcversion"
	"google.golang.org/grpc"
)

// NewConn creates a gRPC client connection to the Uncloud API at target. It adds the options every Uncloud API
// client needs, such as sending the client version that the daemon checks, to the given opts. Interceptors from
// opts run before the Uncloud ones.
func NewConn(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	// Chained interceptors run last and can't be replaced by grpc.WithUnaryInterceptor or
	// grpc.WithStreamInterceptor in opts.
	opts = slices.Concat(opts, []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(grpcversion.ClientUnaryInterceptor),
		grpc.WithChainStreamInterceptor(grpcversion.ClientStreamInterceptor),
	})
	return grpc.NewClient(target, opts...)
}
