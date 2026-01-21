package api

import (
	"testing"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/stretchr/testify/assert"
)

func TestVolumeSpec_MatchesDockerVolume(t *testing.T) {
	t.Parallel()

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
		{
			name: "non-match with different driver options",
			spec: VolumeSpec{
				Type: VolumeTypeVolume,
				VolumeOptions: &VolumeOptions{
					Driver: &mount.Driver{
						Name: "local",
						Options: map[string]string{
							"foo": "baz",
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
			expected: false,
		},
		{
			name: "non-match with different driver name",
			spec: VolumeSpec{
				Type: VolumeTypeVolume,
				VolumeOptions: &VolumeOptions{
					Driver: &mount.Driver{
						Name: "custom",
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
			expected: false,
		},
		{
			name: "match external volume without driver by name only",
			spec: VolumeSpec{
				Name: "external",
				Type: VolumeTypeVolume,
			},
			vol: volume.Volume{
				Name:   "external",
				Driver: "local",
				Options: map[string]string{
					"foo": "bar",
				},
			},
			expected: true,
		},
		{
			name: "non-match external volume by name",
			spec: VolumeSpec{
				Name: "unknown",
				Type: VolumeTypeVolume,
			},
			vol: volume.Volume{
				Name:   "external",
				Driver: "local",
				Options: map[string]string{
					"foo": "bar",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := tt.spec.MatchesDockerVolume(tt.vol)
			assert.Equal(t, tt.expected, matches)
		})
	}
}
