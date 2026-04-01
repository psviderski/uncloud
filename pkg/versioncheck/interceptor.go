package versioncheck

import (
	"context"
	"fmt"
	"os"

	"github.com/Masterminds/semver"
	internalVersion "github.com/psviderski/uncloud/internal/version"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	MetadataKeyCLIVersion       = "uncloud-client-version"
	MetadataKeyMinDaemonVersion = "uncloud-min-server-version"
	MetadataKeyDaemonVersion    = "uncloud-server-version"

	// MinCLIVersion is the minimum client version the daemon accepts. The daemon
	// rejects requests from older clients, forcing them to upgrade. This provides
	// a clean cut-off for dropping support for old clients.
	//
	// MinDaemonVersion is the minimum daemon version the client requires. The client
	// sends this with each request so the daemon can immediately reject if it's too old,
	// avoiding the need for a preflight request. This is useful when a new client feature
	// requires daemon capabilities that didn't exist in older versions.
	//
	// The two minimums are independent: a client might require a newer daemon for new
	// features, while that same daemon could still handle requests from older clients.
	MinCLIVersion    = "0.0.0"
	MinDaemonVersion = "0.0.0"

	ReleaseURL = "https://github.com/psviderski/uncloud/releases/latest"
)

var (
	// currentVersion is the version of this binary (CLI or daemon)
	currentVersion = semver.MustParse(internalVersion.String())
	// zeroVersion is used when no version is specified (treated as 0.0.0)
	zeroVersion = semver.MustParse("0.0.0")
	// Pre-parsed minimum versions for comparison
	minCLIVersion    = semver.MustParse(MinCLIVersion)
	minDaemonVersion = semver.MustParse(MinDaemonVersion)

	// warned tracks if we've already printed the daemon version warning
	// TODO: remove when checkDaemonVersionInResponse is no longer needed (see below)
	warned bool
)

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

	actualCLIVersion := extractVersion(md, MetadataKeyCLIVersion)
	if actualCLIVersion.LessThan(minCLIVersion) {
		return status.Errorf(codes.FailedPrecondition,
			"version check failed: client version is below minimum %s. Please upgrade: %s",
			minCLIVersion, ReleaseURL)
	}

	requiredMinDaemon := extractVersion(md, MetadataKeyMinDaemonVersion)
	if currentVersion.LessThan(requiredMinDaemon) {
		return status.Errorf(codes.FailedPrecondition,
			"version check failed: daemon version %s is below client's minimum required version %s. Please upgrade the daemon: %s",
			currentVersion, requiredMinDaemon, ReleaseURL)
	}

	return nil
}

func ServerUnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if err := checkClientVersionHeaders(ctx); err != nil {
		return nil, err
	}
	if err := grpc.SetHeader(ctx, metadata.Pairs(MetadataKeyDaemonVersion, currentVersion.String())); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

func ServerStreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := checkClientVersionHeaders(ss.Context()); err != nil {
		return err
	}
	if err := ss.SetHeader(metadata.Pairs(MetadataKeyDaemonVersion, currentVersion.String())); err != nil {
		return err
	}
	return handler(srv, ss)
}

func ClientUnaryInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	ctx = metadata.AppendToOutgoingContext(ctx,
		MetadataKeyCLIVersion, currentVersion.String(),
		MetadataKeyMinDaemonVersion, MinDaemonVersion,
	)

	// TODO: remove when checkDaemonVersionInResponse is no longer needed,
	// as we'll no longer need to extract headers from the response here.
	var respMD metadata.MD
	opts = append(opts, grpc.Header(&respMD))

	err := invoker(ctx, method, req, reply, cc, opts...)
	if err != nil {
		return err
	}

	// TODO: Remove eventually (see note on method below)
	checkDaemonVersionInResponse(respMD)

	return nil
}

// This is just needed as a warning during the transition to version checking
// releases. It warns the user when they just communicated with a daemon that did
// not check the version requirements.
// TODO: Remove this in some later release, after users have upgraded.
func checkDaemonVersionInResponse(md metadata.MD) {
	daemonVersion := extractVersion(md, MetadataKeyDaemonVersion)
	if daemonVersion.LessThan(minDaemonVersion) {
		if warned {
			return
		}
		warned = true

		msg := fmt.Sprintf("daemon version is below minimum required version %s. The daemon did not verify this CLI's minimum version requirement, so the operation may not have behaved as intended. Please upgrade the daemon: %s",
			minDaemonVersion, ReleaseURL)
		fmt.Fprintf(os.Stderr, "WARNING: %s\n", msg)
	}
}

func ClientStreamInterceptor(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	ctx = metadata.AppendToOutgoingContext(ctx,
		MetadataKeyCLIVersion, currentVersion.String(),
		MetadataKeyMinDaemonVersion, MinDaemonVersion,
	)

	stream, err := streamer(ctx, desc, cc, method, opts...)
	if err != nil {
		return nil, err
	}

	// TODO: Wrapping the stream in versionedClientStream will no longer
	// be necessary when we are ready to remove the temporary, transition
	// safety check checkDaemonVersionInResponse (see note on method above)
	return &versionedClientStream{ClientStream: stream}, nil
}

// TODO: remove when checkDaemonVersionInResponse is no longer needed
type versionedClientStream struct {
	grpc.ClientStream
}

// TODO: remove when checkDaemonVersionInResponse is no longer needed
func (s *versionedClientStream) Header() (metadata.MD, error) {
	md, err := s.ClientStream.Header()
	if err != nil {
		return nil, err
	}

	checkDaemonVersionInResponse(md)

	return md, nil
}
