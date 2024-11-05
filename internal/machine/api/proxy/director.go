package proxy

import (
	"context"
	"github.com/siderolabs/grpc-proxy/proxy"
	"sync"
)

// Director manages routing of gRPC requests between local and remote backends.
type Director struct {
	localTarget    string
	localBackend   proxy.Backend
	remoteBackends sync.Map

	mu sync.RWMutex
}

func NewDirector(localSockPath string) *Director {
	return &Director{
		localBackend: NewLocalBackend(localSockPath),
	}
}

// UpdateLocalAddress updates the local machine address used to identify which requests should be proxied
// to the local gRPC server.
func (d *Director) UpdateLocalAddress(target string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.localTarget = target
}

// Director implements proxy.StreamDirector for grpc-proxy, routing requests to local or remote backends based
// on gRPC metadata in the context.
func (d *Director) Director(ctx context.Context, fullMethodName string) (proxy.Mode, []proxy.Backend, error) {
	return proxy.One2One, []proxy.Backend{d.localBackend}, nil
}
