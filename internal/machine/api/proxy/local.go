package proxy

import (
	"context"
	"github.com/siderolabs/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"sync"
)

// LocalBackend is a proxy.Backend implementation that proxies to a local gRPC server listening on a Unix socket.
type LocalBackend struct {
	sockPath string

	mu   sync.RWMutex
	conn *grpc.ClientConn
}

var _ proxy.Backend = (*LocalBackend)(nil)

// NewLocalBackend returns a new LocalBackend for the given Unix socket path.
func NewLocalBackend(sockPath string) *LocalBackend {
	return &LocalBackend{
		sockPath: sockPath,
	}
}

func (l *LocalBackend) String() string {
	return "local"
}

// GetConnection returns a gRPC connection to the local server listening on the Unix socket.
func (l *LocalBackend) GetConnection(ctx context.Context, _ string) (context.Context, *grpc.ClientConn, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	outCtx := metadata.NewOutgoingContext(ctx, md)

	l.mu.RLock()
	if l.conn != nil {
		l.mu.RUnlock()
		return outCtx, l.conn, nil
	}
	l.mu.RUnlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	var err error
	l.conn, err = grpc.NewClient(
		"unix://"+l.sockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodecV2(proxy.Codec()),
		),
	)

	return outCtx, l.conn, err
}

// AppendInfo is called to enhance response from the backend with additional data.
func (l *LocalBackend) AppendInfo(_ bool, resp []byte) ([]byte, error) {
	return resp, nil
}

// BuildError is called to convert error from upstream into response field.
func (l *LocalBackend) BuildError(bool, error) ([]byte, error) {
	return nil, nil
}
