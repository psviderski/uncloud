package proxy

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/siderolabs/grpc-proxy/proxy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Director manages routing of gRPC requests between local and remote backends.
type Director struct {
	localBackend   *LocalBackend
	remotePort     uint16
	remoteBackends sync.Map
	localAddress   string
	mapper         MachineMapper
}

func NewDirector(localSockPath string, remotePort uint16, mapper MachineMapper) *Director {
	return &Director{
		localBackend: NewLocalBackend(localSockPath),
		remotePort:   remotePort,
		mapper:       mapper,
	}
}

// UpdateLocalAddress updates the local machine address used to identify which requests should be proxied
// to the local gRPC server
func (d *Director) UpdateLocalAddress(addr string) {
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
	machines, hasMachines := md["machines"]
	machine, hasMachine := md["machine"]
	if !hasMachines && !hasMachine {
		return proxy.One2One, []proxy.Backend{d.localBackend}, nil
	}

	// Handle singular "machine" case (One2One, no metadata injection)
	if hasMachine && len(machine) > 0 {
		name := machine[0]
		targets, err := d.mapper.MapMachines(ctx, []string{name})
		if err != nil {
			return proxy.One2One, nil, status.Error(codes.InvalidArgument, fmt.Sprintf("failed to resolve machine %s: %v", name, err))
		}
		if len(targets) != 1 {
			return proxy.One2One, nil, status.Error(codes.InvalidArgument, fmt.Sprintf("machine not found: %s", name))
		}

		backend, err := d.getBackend(targets[0].Addr)
		if err != nil {
			return proxy.One2One, nil, status.Error(codes.Internal, err.Error())
		}

		// For One2One, we don't wrap in MetadataBackend as we don't inject metadata.
		return proxy.One2One, []proxy.Backend{backend}, nil
	}

	// Handle plural "machines" case (One2Many, always metadata injection)
	if len(machines) == 0 {
		return proxy.One2One, nil, status.Error(codes.InvalidArgument, "no machines specified")
	}

	targets, err := d.mapper.MapMachines(ctx, machines)
	if err != nil {
		return proxy.One2One, nil, status.Error(codes.Internal, fmt.Sprintf("failed to resolve machines: %v", err))
	}
	// Skip length check for wildcard "*" which returns all machines.
	if !slices.Contains(machines, "*") && len(targets) != len(machines) {
		// TODO: identify which specific machine name/ID did not match.
		return proxy.One2One, nil, status.Error(codes.InvalidArgument, "some machines not found")
	}

	backends := make([]proxy.Backend, len(targets))
	for i, t := range targets {
		backend, err := d.getBackend(t.Addr)
		if err != nil {
			return proxy.One2One, nil, status.Error(codes.Internal, err.Error())
		}

		// Wrap with metadata injector
		backends[i] = &MetadataBackend{
			Backend:     backend,
			MachineID:   t.ID,
			MachineName: t.Name,
			MachineAddr: t.Addr,
		}
	}

	return proxy.One2Many, backends, nil
}

// getBackend returns a backend for the given address, utilizing local backend if matching local address.
func (d *Director) getBackend(addr string) (proxy.Backend, error) {
	if addr == d.localAddress {
		return d.localBackend, nil
	}
	return d.remoteBackend(addr)
}

// remoteBackend returns a RemoteBackend for the given address from the cache or creates a new one.
func (d *Director) remoteBackend(addr string) (*RemoteBackend, error) {
	b, ok := d.remoteBackends.Load(addr)
	if ok {
		return b.(*RemoteBackend), nil
	}

	backend, err := NewRemoteBackend(addr, d.remotePort)
	if err != nil {
		return nil, err
	}
	existing, loaded := d.remoteBackends.LoadOrStore(addr, backend)
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
