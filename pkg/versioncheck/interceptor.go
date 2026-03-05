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
	MetadataKeyCLIVersion       = "x-uncloud-cli-version"
	MetadataKeyMinDaemonVersion = "x-uncloud-min-daemon-version"
	MetadataKeyDaemonVersion    = "x-uncloud-daemon-version"

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

	respMD, _ := metadata.FromIncomingContext(ctx)
	checkDaemonVersionInResponse(respMD)

	return nil
}

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
