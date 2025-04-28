package dns

import (
	"context"
	"net/netip"
)

// Server provides a DNS server for service discovery and proxying to upstream DNS servers.
type Server struct {
	store RecordsStore
}

type RecordsStore interface {
	Resolve(ctx context.Context, name string) ([]netip.Addr, error)
}
