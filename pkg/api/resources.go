package api

import (
	"github.com/docker/docker/api/types/container"
)

const (
	MilliCore = 1_000_000
	Core      = 1000 * MilliCore
)

type ContainerResources struct {
	// CPU is the maximum amount of CPU nanocores (1000000000 = 1 CPU core) the container can use.
	CPU int64
	// Memory is the maximum amount of memory (in bytes) the container can use.
	Memory int64
	// MemoryReservation is the minimum amount of memory (in bytes) the container needs to run efficiently.
	// TODO: implement a placement constraint that checks available memory on machines.
	MemoryReservation int64
	// Devices provides direct access to host devices.
	Devices []DeviceMapping
	// DeviceReservations requests for access to things like GPUs.
	DeviceReservations []container.DeviceRequest
	// Ulimits defines the resource limits for the container.
	Ulimits map[string]Ulimit
}

// DeviceMapping represents a device mapping between host and container.
type DeviceMapping struct {
	// HostPath is the path to the device on the host.
	HostPath string
	// ContainerPath is the path to the device in the container.
	ContainerPath string
	// CgroupPermissions is the cgroup permissions for the device (e.g., "rwm").
	CgroupPermissions string
}

type Ulimit struct {
	Soft int64
	Hard int64
}
