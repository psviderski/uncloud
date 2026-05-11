package grpcversion

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync/atomic"

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
	MetadataKeyServerVersion    = "uncloud-server-version"

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
	MinClientVersion = "0.0.0"
	MinServerVersion = "0.0.0"

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

	// warned tracks if we've already printed the daemon version warning.
	// TODO: Remove when checkServerVersionInResponse is no longer needed (see below).
	warned atomic.Bool

	// WarnWriter is the writer used for version mismatch warnings. Defaults to os.Stderr.
	// Tests can override this to capture warning output.
	WarnWriter io.Writer = os.Stderr
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
			"version check failed: daemon version %s is below client's minimum required version %s. Please upgrade the daemon: %s",
			currentVersion, requiredMinServer, ReleaseURL)
	}

	return nil
}

func ServerUnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if err := checkClientVersionHeaders(ctx); err != nil {
		return nil, err
	}
	if err := grpc.SetHeader(ctx, metadata.Pairs(MetadataKeyServerVersion, currentVersion.String())); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

func ServerStreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := checkClientVersionHeaders(ss.Context()); err != nil {
		return err
	}
	if err := ss.SetHeader(metadata.Pairs(MetadataKeyServerVersion, currentVersion.String())); err != nil {
		return err
	}
	return handler(srv, ss)
}

func ClientUnaryInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	ctx = metadata.AppendToOutgoingContext(ctx,
		MetadataKeyClientVersion, currentVersion.String(),
		MetadataKeyMinServerVersion, MinServerVersion,
	)

	// TODO: Remove when checkServerVersionInResponse is no longer needed,
	// as we'll no longer need to extract headers from the response here.
	var respMD metadata.MD
	opts = append(opts, grpc.Header(&respMD))

	err := invoker(ctx, method, req, reply, cc, opts...)
	if err != nil {
		return err
	}

	// TODO: Remove eventually (see note on method below).
	checkServerVersionInResponse(respMD)

	return nil
}

// checkServerVersionInResponse warns the user when they communicated with a daemon that
// did not check the version requirements. This is only needed during the transition to
// version-checking releases.
// TODO: Remove this in some later release, after users have upgraded.
func checkServerVersionInResponse(md metadata.MD) {
	serverVersion := extractVersion(md, MetadataKeyServerVersion)
	if serverVersion.LessThan(minServerVersion) {
		if warned.Swap(true) {
			return
		}

		msg := fmt.Sprintf("daemon version is below minimum required version %s. The daemon did not verify this CLI's minimum version requirement, so the operation may not have behaved as intended. Please upgrade the daemon: %s",
			minServerVersion, ReleaseURL)
		fmt.Fprintf(WarnWriter, "WARNING: %s\n", msg)
	}
}

func ClientStreamInterceptor(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	ctx = metadata.AppendToOutgoingContext(ctx,
		MetadataKeyClientVersion, currentVersion.String(),
		MetadataKeyMinServerVersion, MinServerVersion,
	)

	stream, err := streamer(ctx, desc, cc, method, opts...)
	if err != nil {
		return nil, err
	}

	// TODO: Wrapping the stream in versionedClientStream will no longer
	// be necessary when we are ready to remove the temporary, transition
	// safety check checkServerVersionInResponse (see note on method above).
	return &versionedClientStream{ClientStream: stream}, nil
}

// TODO: Remove when checkServerVersionInResponse is no longer needed.
type versionedClientStream struct {
	grpc.ClientStream
}

// TODO: Remove when checkServerVersionInResponse is no longer needed.
func (s *versionedClientStream) Header() (metadata.MD, error) {
	md, err := s.ClientStream.Header()
	if err != nil {
		return nil, err
	}

	checkServerVersionInResponse(md)

	return md, nil
}
