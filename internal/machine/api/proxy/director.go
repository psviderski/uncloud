package proxy

import (
	"context"
	"fmt"
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
	// mu synchronizes access to localAddress.
	mu           sync.RWMutex
	localAddress string
	localID      string
	localName    string
	directory    MachineDirectory
}

func NewDirector(localSockPath string, remotePort uint16, directory MachineDirectory) *Director {
	return &Director{
		localBackend: NewLocalBackend(localSockPath, "", "", ""),
		remotePort:   remotePort,
		directory:    directory,
	}
}

// UpdateLocalMachine updates the local machine details used to identify which requests should be proxied
// to the local gRPC server.
func (d *Director) UpdateLocalMachine(id, name, addr string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.localAddress = addr
	d.localID = id
	d.localName = name
	// Replace the local backend with the one that has local address set.
	d.localBackend = NewLocalBackend(d.localBackend.sockPath, addr, id, name)
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

	d.mu.RLock()
	localAddress := d.localAddress
	localBackend := d.localBackend
	d.mu.RUnlock()

	type target struct {
		id, name, addr string
	}
	var targets []target

	// Handle singular "machine" case (One2One, no metadata injection)
	if hasMachine && len(machine) > 0 {
		name := machine[0]
		id, mName, ip, err := d.directory.ResolveMachine(ctx, name)
		if err != nil {
			return proxy.One2One, nil, status.Error(codes.InvalidArgument, fmt.Sprintf("failed to resolve machine %s: %v", name, err))
		}

		t := target{id: id, name: mName, addr: ip.String()}
		var backend proxy.Backend
		if t.addr == localAddress {
			backend = localBackend
		} else {
			backend, err = d.remoteBackend(t.addr, t.id, t.name)
			if err != nil {
				return proxy.One2One, nil, status.Error(codes.Internal, err.Error())
			}
		}
		return proxy.One2One, []proxy.Backend{backend}, nil
	}

	// Handle plural "machines" case (One2Many, always metadata injection)
	if len(machines) == 0 {
		return proxy.One2One, nil, status.Error(codes.InvalidArgument, "no machines specified")
	}

	targets = make([]target, 0, len(machines))

	// Check for "all" machines wildcard
	proxyAll := false
	for _, name := range machines {
		if name == "*" {
			proxyAll = true
			break
		}
	}

	if proxyAll {
		allMachines, err := d.directory.ListMachines(ctx)
		if err != nil {
			return proxy.One2One, nil, status.Error(codes.Internal, fmt.Sprintf("failed to list machines: %v", err))
		}
		for _, m := range allMachines {
			ip, err := m.Network.ManagementIp.ToAddr()
			if err != nil {
				continue
			}
			targets = append(targets, target{id: m.Id, name: m.Name, addr: ip.String()})
		}
	} else {
		for _, name := range machines {
			id, mName, ip, err := d.directory.ResolveMachine(ctx, name)
			if err != nil {
				return proxy.One2One, nil, status.Error(codes.InvalidArgument, fmt.Sprintf("failed to resolve machine %s: %v", name, err))
			}
			targets = append(targets, target{id: id, name: mName, addr: ip.String()})
		}
	}

	backends := make([]proxy.Backend, len(targets))
	for i, t := range targets {
		if t.addr == localAddress {
			backends[i] = localBackend
			continue
		}

		backend, err := d.remoteBackend(t.addr, t.id, t.name)
		if err != nil {
			return proxy.One2One, nil, status.Error(codes.Internal, err.Error())
		}
		backends[i] = backend
	}

	return proxy.One2Many, backends, nil
}

// remoteBackend returns a RemoteBackend for the given address from the cache or creates a new one.
func (d *Director) remoteBackend(addr, id, name string) (*RemoteBackend, error) {
	b, ok := d.remoteBackends.Load(addr)
	if ok {
		return b.(*RemoteBackend), nil
	}

	backend, err := NewRemoteBackend(addr, d.remotePort, id, name)
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
