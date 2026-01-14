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
	// Device reservations/requests for access to things like GPUs
	DeviceReservations []container.DeviceRequest
}

type Ulimit struct {
	Name string
	Soft int64
	Hard int64
}
