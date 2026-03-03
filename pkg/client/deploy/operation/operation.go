package operation

import (
	"context"

	"github.com/psviderski/uncloud/pkg/api"
)

// Operation represents a single atomic operation in a deployment process.
// Operations can be composed to form complex deployment strategies.
type Operation interface {
	// Execute performs the operation using the provided client.
	Execute(ctx context.Context, cli Client) error
	// Format returns a human-readable representation of the operation.
	// TODO: get rid of the resolver and assign the required names for formatting in the operation itself.
	Format(resolver NameResolver) string
	String() string
}

// Client defines the interface required to execute deployment operations.
type Client interface {
	api.ContainerClient
	api.VolumeClient
}

// NameResolver resolves machine and container IDs to their names.
type NameResolver interface {
	MachineName(machineID string) string
	ContainerName(containerID string) string
}
