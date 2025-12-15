package api

import (
	"testing"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/stretchr/testify/assert"
)

func TestVolumeSpec_MatchesDockerVolume(t *testing.T) {
	tests := []struct {
		name     string
		spec     VolumeSpec
		vol      volume.Volume
		expected bool
	}{
		{
			name: "match with explicit local driver",
			spec: VolumeSpec{
				Type: VolumeTypeVolume,
				VolumeOptions: &VolumeOptions{
					Driver: &mount.Driver{
						Name: "local",
						Options: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			vol: volume.Volume{
				Driver: "local",
				Options: map[string]string{
					"foo": "bar",
				},
			},
			expected: true,
		},
		{
			name: "match with empty driver name in spec (implicit local)",
			spec: VolumeSpec{
				Type: VolumeTypeVolume,
				VolumeOptions: &VolumeOptions{
					Driver: &mount.Driver{
						Name: "", // Implicitly local
						Options: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			vol: volume.Volume{
				Driver: "local",
				Options: map[string]string{
					"foo": "bar",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := tt.spec.MatchesDockerVolume(tt.vol)
			assert.Equal(t, tt.expected, matches)
		})
	}
}
