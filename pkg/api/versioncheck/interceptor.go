package versioncheck

import (
	"context"
	"log/slog"

	"github.com/Masterminds/semver/v3"
	internalVersion "github.com/psviderski/uncloud/internal/version"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	MetadataKeyCLIVersion       = "x-uncloud-cli-version"
	MetadataKeyMinDaemonVersion = "x-uncloud-min-daemon-version"
	MetadataKeyDaemonVersion    = "x-uncloud-daemon-version"

	MinCLIVersion    = "0.0.0"
	MinDaemonVersion = "0.0.0"

	ReleaseURL = "https://github.com/psviderski/uncloud/releases/latest"
)

var (
	// currentVersion is the version of this binary (CLI or daemon)
	currentVersion = mustParseVersion(internalVersion.String())
	// zeroVersion is used when no version is specified (treated as 0.0.0)
	zeroVersion = semver.MustParse("0.0.0")
	// Pre-parsed minimum versions for comparison
	minCLIVersion    = mustParseVersion(MinCLIVersion)
	minDaemonVersion = mustParseVersion(MinDaemonVersion)
)

func mustParseVersion(v string) *semver.Version {
	sv, err := semver.NewVersion(v)
	if err != nil {
		panic("invalid hardcoded version constant: " + v)
	}
	return sv
}

func parseVersion(values []string) *semver.Version {
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
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Internal, "version check failed: no gRPC metadata in context")
	}

	actualCLIVersion := parseVersion(md.Get(MetadataKeyCLIVersion))
	if actualCLIVersion.LessThan(minCLIVersion) {
		return status.Errorf(codes.FailedPrecondition,
			"version check failed: client version %s is below minimum %s. Please upgrade: %s",
			actualCLIVersion, minCLIVersion, ReleaseURL)
	}

	requiredMinDaemon := parseVersion(md.Get(MetadataKeyMinDaemonVersion))
	if currentVersion.LessThan(requiredMinDaemon) {
		return status.Errorf(codes.FailedPrecondition,
			"version check failed: daemon version %s is below client's minimum required version %s. Please upgrade the daemon: %s",
			currentVersion, requiredMinDaemon, ReleaseURL)
	}

	return nil
}

func ServerUnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if err := checkClientVersionHeaders(ctx); err != nil {
		return nil, err
	}
	if err := grpc.SetHeader(ctx, metadata.Pairs(MetadataKeyDaemonVersion, currentVersion.String())); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

func ServerStreamInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := checkClientVersionHeaders(ss.Context()); err != nil {
		return err
	}
	if err := grpc.SetHeader(ss.Context(), metadata.Pairs(MetadataKeyDaemonVersion, currentVersion.String())); err != nil {
		return err
	}
	return handler(srv, ss)
}

func ClientUnaryInterceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	md := metadata.Pairs(
		MetadataKeyCLIVersion, currentVersion.String(),
		MetadataKeyMinDaemonVersion, MinDaemonVersion,
	)
	ctx = metadata.NewOutgoingContext(ctx, md)

	err := invoker(ctx, method, req, reply, cc, opts...)
	if err != nil {
		return err
	}

	respMD, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Internal, "version check failed: no gRPC metadata in response context")
	}

	checkDaemonVersionInResponse(respMD)

	return nil
}

func checkDaemonVersionInResponse(md metadata.MD) {
	daemonVersions := md.Get(MetadataKeyDaemonVersion)
	if len(daemonVersions) == 0 || daemonVersions[0] == "" {
		slog.Warn("daemon response missing version metadata - please upgrade daemon",
			"upgrade_url", ReleaseURL)
	}
}

func ClientStreamInterceptor(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	md := metadata.Pairs(
		MetadataKeyCLIVersion, currentVersion.String(),
		MetadataKeyMinDaemonVersion, MinDaemonVersion,
	)
	ctx = metadata.NewOutgoingContext(ctx, md)

	stream, err := streamer(ctx, desc, cc, method, opts...)
	if err != nil {
		return nil, err
	}

	return &versionedClientStream{ClientStream: stream}, nil
}

type versionedClientStream struct {
	grpc.ClientStream
}

func (s *versionedClientStream) Header() (metadata.MD, error) {
	md, err := s.ClientStream.Header()
	if err != nil {
		return nil, err
	}

	checkDaemonVersionInResponse(md)

	return md, nil
}
