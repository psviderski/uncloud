package proxy

import (
	"context"
	"github.com/siderolabs/grpc-proxy/proxy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"net"
	"strconv"
	"sync"
)

// Director manages routing of gRPC requests between local and remote backends.
type Director struct {
	localBackend   *LocalBackend
	remotePort     int
	remoteBackends sync.Map
	// mu synchronizes access to localAddress.
	mu           sync.RWMutex
	localAddress string
}

func NewDirector(localSockPath string, remotePort int) *Director {
	return &Director{
		localBackend: NewLocalBackend(localSockPath),
		remotePort:   remotePort,
	}
}

// UpdateLocalAddress updates the local machine address used to identify which requests should be proxied
// to the local gRPC server.
func (d *Director) UpdateLocalAddress(addr string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.localAddress = addr
}

// Director implements proxy.StreamDirector for grpc-proxy, routing requests to local or remote backends based
// on gRPC metadata in the context. Each machine metadata is injected into the response messages by the proxy
// if the request is proxied to multiple backends.
func (d *Director) Director(ctx context.Context, fullMethodName string) (proxy.Mode, []proxy.Backend, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return proxy.One2One, []proxy.Backend{d.localBackend}, nil
	}
	// If the request is already proxied, send it to the local backend.
	if _, ok = md["proxy-authority"]; ok {
		return proxy.One2One, []proxy.Backend{d.localBackend}, nil
	}
	// If the request metadata doesn't contain machines to proxy to, send it to the local backend.
	machines, ok := md["machines"]
	if !ok {
		return proxy.One2One, []proxy.Backend{d.localBackend}, nil
	}
	if len(machines) == 0 {
		return proxy.One2One, nil, status.Error(codes.InvalidArgument, "no machines specified")
	}

	d.mu.RLock()
	localAddress := d.localAddress
	d.mu.RUnlock()

	backends := make([]proxy.Backend, len(machines))
	for i, addr := range machines {
		if addr == localAddress {
			backends[i] = d.localBackend
			continue
		}

		target := net.JoinHostPort(addr, strconv.Itoa(d.remotePort))
		backend, err := d.remoteBackend(target)
		if err != nil {
			return proxy.One2One, nil, status.Error(codes.Internal, err.Error())
		}
		backends[i] = backend
	}

	if len(backends) == 1 {
		return proxy.One2One, backends, nil
	}
	return proxy.One2Many, backends, nil
}

// remoteBackend returns a RemoteBackend for the given target from the cache or creates a new one.
func (d *Director) remoteBackend(target string) (*RemoteBackend, error) {
	b, ok := d.remoteBackends.Load(target)
	if ok {
		return b.(*RemoteBackend), nil
	}

	backend, err := NewRemoteBackend(target)
	if err != nil {
		return nil, err
	}
	existing, loaded := d.remoteBackends.LoadOrStore(target, backend)
	if loaded {
		// A concurrent remoteBackend call built a different backend.
		backend.Close()
		return existing.(*RemoteBackend), nil
	}

	return backend, nil
}

// FlushRemoteBackends closes all remote backend connections and removes them from the cache.
func (d *Director) FlushRemoteBackends() {
	d.remoteBackends.Range(func(key, value interface{}) bool {
		backend, ok := value.(*RemoteBackend)
		if !ok {
			return true
		}

		backend.Close()
		d.remoteBackends.Delete(key)
		return true
	})
}

// Close closes all backend connections.
func (d *Director) Close() {
	d.localBackend.Close()
	d.FlushRemoteBackends()
}
