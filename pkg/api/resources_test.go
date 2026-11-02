package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContainerResources_ValidateUpdateInPlace(t *testing.T) {
	t.Parallel()

	current := ContainerResources{
		CPU:               Core,
		Memory:            512 * 1024 * 1024,
		MemoryReservation: 256 * 1024 * 1024,
	}

	tests := []struct {
		name    string
		new     func(r *ContainerResources)
		wantErr string
	}{
		{
			name: "same resources",
			new:  func(*ContainerResources) {},
		},
		{
			name: "change CPU and memory limits",
			new: func(r *ContainerResources) {
				r.CPU = 2 * Core
				r.Memory = 1024 * 1024 * 1024
				r.MemoryReservation = 128 * 1024 * 1024
			},
		},
		{
			name:    "remove CPU limit",
			new:     func(r *ContainerResources) { r.CPU = 0 },
			wantErr: "CPU limit can't be removed",
		},
		{
			name:    "remove memory limit",
			new:     func(r *ContainerResources) { r.Memory = 0 },
			wantErr: "memory limit can't be removed",
		},
		{
			name:    "remove memory reservation",
			new:     func(r *ContainerResources) { r.MemoryReservation = 0 },
			wantErr: "memory reservation can't be removed",
		},
		{
			name:    "change shared memory",
			new:     func(r *ContainerResources) { r.SharedMemory = 64 * 1024 * 1024 },
			wantErr: "only CPU and memory limits can be changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			newResources := current
			tt.new(&newResources)

			err := current.ValidateUpdateInPlace(newResources)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr)
			}
		})
	}
}
