package api

import (
	"errors"

	"github.com/docker/docker/api/types/container"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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
	// SharedMemory is the size of the shared memory (in bytes) mounted at /dev/shm in the container.
	SharedMemory int64
	// Ulimits defines the resource limits for the container.
	Ulimits map[string]Ulimit
}

// Equals returns true if the resources are equal, treating nil and empty slices and maps as equal.
func (r ContainerResources) Equals(other ContainerResources) bool {
	return cmp.Equal(r, other, cmpopts.EquateEmpty())
}

// ValidateUpdateInPlace returns an error explaining why a container created with these resources can't be updated
// to the new resources without recreating it. It returns nil if the container can be updated in place.
// Docker can only change the CPU and memory limits of an existing container and can't remove a limit once it's set.
func (r ContainerResources) ValidateUpdateInPlace(new ContainerResources) error {
	// Docker treats zero values as unchanged when updating a container.
	if r.CPU != 0 && new.CPU == 0 {
		return errors.New("CPU limit can't be removed")
	}
	if r.Memory != 0 && new.Memory == 0 {
		return errors.New("memory limit can't be removed")
	}
	if r.MemoryReservation != 0 && new.MemoryReservation == 0 {
		return errors.New("memory reservation can't be removed")
	}

	r.CPU = new.CPU
	r.Memory = new.Memory
	r.MemoryReservation = new.MemoryReservation
	if !r.Equals(new) {
		return errors.New("only CPU and memory limits can be changed")
	}
	return nil
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
