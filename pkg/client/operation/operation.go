package operation

import (
	"context"

	"github.com/psviderski/uncloud/pkg/api"
)

// Operation represents a single operation in the cluster that can be executed.
// Operations can be composed to form complex workflows (plans).
type Operation interface {
	// Execute performs the operation using the provided client.
	Execute(ctx context.Context, cli Client) error
	// Format returns a human-readable representation of the operation.
	Format() string
	String() string
}

// Client defines the interface required to execute cluster operations.
type Client interface {
	api.ContainerClient
	api.VolumeClient
}
