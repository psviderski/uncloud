package proxy

import (
	"context"
	"sync"

	"github.com/siderolabs/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
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

func (b *LocalBackend) String() string {
	return "unix://" + b.sockPath
}

// GetConnection returns a gRPC connection to the local server listening on the Unix socket.
func (b *LocalBackend) GetConnection(ctx context.Context, _ string) (context.Context, *grpc.ClientConn, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	outCtx := metadata.NewOutgoingContext(ctx, md)

	b.mu.RLock()
	if b.conn != nil {
		defer b.mu.RUnlock()
		return outCtx, b.conn, nil
	}
	b.mu.RUnlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	var err error
	b.conn, err = grpc.NewClient(
		"unix://"+b.sockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodecV2(proxy.Codec()),
		),
	)

	return outCtx, b.conn, err
}

// AppendInfo is a no-op for LocalBackend as it does not inject metadata.
func (b *LocalBackend) AppendInfo(streaming bool, resp []byte) ([]byte, error) {
	return resp, nil
}

// BuildError is a no-op for LocalBackend.
func (b *LocalBackend) BuildError(streaming bool, err error) ([]byte, error) {
	return nil, err
}

// Close closes the upstream gRPC connection.
func (b *LocalBackend) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
	}
}
