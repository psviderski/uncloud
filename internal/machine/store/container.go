package store

import (
	"time"
	"uncloud/internal/machine/docker/container"
)

const (
	// SyncStatusSynced indicates that a container record is synchronised with the Docker daemon. The record may
	// become outdated even when the status is "synced" if the machine crashes or a network partition occurs.
	// The cluster membership state of the machine should also be checked to determine if the record can be trusted.
	SyncStatusSynced = "synced"
	// SyncStatusOutdated indicates that a container record may be outdated, for example, due to being unable
	// to retrieve the container's state from the Docker daemon or when the machine is being stopped or restarted.
	SyncStatusOutdated = "outdated"
)

type ContainerRecord struct {
	Container  *container.Container
	MachineID  string
	SyncStatus string
	UpdatedAt  time.Time
}
