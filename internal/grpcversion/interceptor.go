// Package grpcversion checks that client and daemon versions are compatible on every gRPC request.
//
// Clients send their version and the minimum daemon version they require with each request. A daemon rejects
// the request if the client is older than MinClientVersion or the daemon is older than the client's minimum.
//
// Daemons should install ServerStreamInterceptor on every server that receives requests from outside the daemon.
// This checks each daemon on the request path once, including the one that routes the request to other machines.
package grpcversion

import (
	"context"

	"github.com/Masterminds/semver"
	"github.com/psviderski/uncloud/internal/version"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	MetadataKeyClientVersion    = "uncloud-client-version"
	MetadataKeyMinServerVersion = "uncloud-min-server-version"
	// MetadataKeyServerVersion is the response header with the daemon version.
	MetadataKeyServerVersion = "uncloud-server-version"

	// MinClientVersion is the minimum client version the daemon accepts. The daemon
	// rejects requests from older clients, forcing them to upgrade. This provides
	// a clean cut-off for dropping support for old clients.
	//
	// MinServerVersion is the minimum daemon version the client requires. The client
	// sends this with each request so the daemon can immediately reject if it's too old,
	// avoiding the need for a preflight request. This is useful when a new client feature
	// requires daemon capabilities that didn't exist in older versions.
	//
	// The two minimums are independent: a client might require a newer daemon for new
	// features, while that same daemon could still handle requests from older clients.
	MinClientVersion = "0.20.0"
	MinServerVersion = "0.20.0"

	ReleaseURL = "https://github.com/psviderski/uncloud/releases/latest"
)

var (
	// zeroVersion is used when no version is specified (treated as 0.0.0).
	zeroVersion = semver.MustParse("0.0.0")
	// currentVersion is the version of this binary (CLI or daemon). It's injected via an ldflag at build time.
	// Fall back to zeroVersion if the injected string isn't valid semver.
	currentVersion = parseVersionOrZero(version.String())
	// Pre-parsed minimum versions for comparison.
	minClientVersion = semver.MustParse(MinClientVersion)
	minServerVersion = semver.MustParse(MinServerVersion)
)

// parseVersionOrZero parses v as a semver, returning zeroVersion if parsing fails.
func parseVersionOrZero(v string) *semver.Version {
	parsed, err := semver.NewVersion(v)
	if err != nil {
		return zeroVersion
	}
	return parsed
}

func extractVersion(md metadata.MD, key string) *semver.Version {
	if md == nil {
		return zeroVersion
	}
	values := md.Get(key)
	if len(values) == 0 || values[0] == "" {
		return zeroVersion
	}
	sv, err := semver.NewVersion(values[0])
	if err != nil {
		return zeroVersion
	}
	return sv
}

func checkClientVersionHeaders(ctx context.Context) error {
	md, _ := metadata.FromIncomingContext(ctx)

	actualClientVersion := extractVersion(md, MetadataKeyClientVersion)
	if actualClientVersion.LessThan(minClientVersion) {
		return status.Errorf(codes.FailedPrecondition,
			"version check failed: client version is below minimum %s. Please upgrade: %s",
			minClientVersion, ReleaseURL)
	}

	requiredMinServer := extractVersion(md, MetadataKeyMinServerVersion)
	if currentVersion.LessThan(requiredMinServer) {
		return status.Errorf(codes.FailedPrecondition,
			"version check failed: daemon version %s is below client's minimum required version %s. "+
				"Please upgrade the daemon: %s",
			currentVersion, requiredMinServer, ReleaseURL)
	}

	return nil
}

// ServerStreamInterceptor rejects requests from clients older than MinClientVersion and requests that require
// a newer daemon version. It reports the daemon version in the response header.
func ServerStreamInterceptor(
	srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler,
) error {
	if err := checkClientVersionHeaders(ss.Context()); err != nil {
		return err
	}
	if err := ss.SetHeader(metadata.Pairs(MetadataKeyServerVersion, currentVersion.String())); err != nil {
		return err
	}
	return handler(srv, ss)
}

// ClientUnaryInterceptor sends the client version and the minimum required daemon version with each unary request.
func ClientUnaryInterceptor(
	ctx context.Context,
	method string,
	req, reply any,
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	return invoker(withVersionMetadata(ctx), method, req, reply, cc, opts...)
}

// ClientStreamInterceptor sends the client version and the minimum required daemon version with each
// streaming request.
func ClientStreamInterceptor(
	ctx context.Context,
	desc *grpc.StreamDesc,
	cc *grpc.ClientConn,
	method string,
	streamer grpc.Streamer,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	return streamer(withVersionMetadata(ctx), desc, cc, method, opts...)
}

// WithMinServerVersion returns a copy of ctx that requires the daemon to be at least the given version
// for requests made with it. The highest of all required versions and MinServerVersion applies.
// It panics if version isn't a valid semantic version.
func WithMinServerVersion(ctx context.Context, version string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, MetadataKeyMinServerVersion, semver.MustParse(version).String())
}

// withVersionMetadata sets the client version and the minimum required daemon version in the outgoing metadata.
func withVersionMetadata(ctx context.Context) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.MD{}
	}

	minServer := minServerVersion
	for _, v := range md.Get(MetadataKeyMinServerVersion) {
		if sv, err := semver.NewVersion(v); err == nil && sv.GreaterThan(minServer) {
			minServer = sv
		}
	}
	// Send each version once because daemons only check the first value.
	md.Set(MetadataKeyClientVersion, currentVersion.String())
	md.Set(MetadataKeyMinServerVersion, minServer.String())

	return metadata.NewOutgoingContext(ctx, md)
}
