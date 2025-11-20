package compose

import (
	"context"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-units"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceSpecFromCompose(t *testing.T) {
	t.Parallel()

	initTrue := true

	tests := []struct {
		name     string
		filename string
		want     map[string]api.ServiceSpec
	}{
		{
			name:     "minimal",
			filename: "compose-minimal.yaml",
			want: map[string]api.ServiceSpec{
				"test": {
					Name: "test",
					Mode: api.ServiceModeReplicated,
					Container: api.ContainerSpec{
						Image:      "nginx:latest",
						PullPolicy: api.PullPolicyMissing,
					},
				},
			},
		},
		{
			name:     "deploy",
			filename: "compose-deploy.yaml",
			want: map[string]api.ServiceSpec{
				"no-deploy": {
					Name: "no-deploy",
					Mode: api.ServiceModeReplicated,
					Container: api.ContainerSpec{
						Image:      "nginx:latest",
						PullPolicy: api.PullPolicyMissing,
						Resources: api.ContainerResources{
							CPU:               1.5 * api.Core,
							Memory:            100 * units.MiB,
							MemoryReservation: 50 * units.MiB,
						},
					},
					Replicas: 3,
				},
				"deploy": {
					Name: "deploy",
					Mode: api.ServiceModeReplicated,
					Container: api.ContainerSpec{
						Image:      "nginx:latest",
						PullPolicy: api.PullPolicyMissing,
						Resources: api.ContainerResources{
							CPU:               1.5 * api.Core,
							Memory:            100 * units.MiB,
							MemoryReservation: 50 * units.MiB,
						},
					},
					Replicas: 3,
				},
				"both": {
					Name: "both",
					Mode: api.ServiceModeReplicated,
					Container: api.ContainerSpec{
						Image:      "nginx:latest",
						PullPolicy: api.PullPolicyMissing,
						Resources: api.ContainerResources{
							CPU:               2 * api.Core,
							Memory:            100 * units.MiB,
							MemoryReservation: 50 * units.MiB,
						},
					},
					Replicas: 3,
				},
			},
		},
		{
			name:     "full-spec",
			filename: "compose-full-spec.yaml",
			want: map[string]api.ServiceSpec{
				"test": {
					Name: "test",
					Mode: api.ServiceModeReplicated,
					Container: api.ContainerSpec{
						Command:    []string{"nginx", "updated", "command"},
						Entrypoint: []string{"/updated-docker-entrypoint.sh"},
						Env: map[string]string{
							"BOOL":  "true",
							"EMPTY": "",
							"VAR":   "value",
						},
						Image: "nginx:latest",
						Init:  &initTrue,
						LogDriver: &api.LogDriver{
							Name: "json-file",
							Options: map[string]string{
								"max-size": "10m",
								"max-file": "3",
							},
						},
						Privileged: true,
						PullPolicy: api.PullPolicyAlways,
						Resources: api.ContainerResources{
							CPU:               0.5 * api.Core,
							Memory:            100 * units.MiB,
							MemoryReservation: 50 * units.MiB,
						},
						User: "nginx:nginx",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "bind-bb6aed1683cea1e0a1ae5cd227aacd0734f2f87f7a78fcf1baeff978ce300b90",
								ContainerPath: "/host/etc/passwd",
								ReadOnly:      true,
							},
							{
								VolumeName:    "data1",
								ContainerPath: "/data1",
							},
							{
								VolumeName:    "data2-alias",
								ContainerPath: "/data2/long/syntax",
							},
							{
								VolumeName:    "data-external",
								ContainerPath: "/external",
								ReadOnly:      true,
							},
							{
								VolumeName:    "tmpfs-efa57ba8b6a1779674ac438de3af8729e2d55900b79eb929431cf9c5b0179542",
								ContainerPath: "/tmpfs",
							},
						},
					},
					Ports: []api.PortSpec{
						{
							Hostname:      "test.example.com",
							ContainerPort: 80,
							Protocol:      api.ProtocolHTTPS,
							Mode:          api.PortModeIngress,
						},
						{
							ContainerPort: 8000,
							Protocol:      api.ProtocolHTTP,
							Mode:          api.PortModeIngress,
						},
						{
							ContainerPort: 3000,
							PublishedPort: 5000,
							Protocol:      "tcp",
							Mode:          api.PortModeHost,
						},
					},
					Replicas: 3,
					Volumes: []api.VolumeSpec{
						{
							Name: "bind-bb6aed1683cea1e0a1ae5cd227aacd0734f2f87f7a78fcf1baeff978ce300b90",
							Type: api.VolumeTypeBind,
							BindOptions: &api.BindOptions{
								HostPath:       "/etc/passwd",
								CreateHostPath: true,
							},
						},
						{
							Name: "data-external",
							Type: api.VolumeTypeVolume,
							VolumeOptions: &api.VolumeOptions{
								Name: "data-external",
							},
						},
						{
							Name: "data1",
							Type: api.VolumeTypeVolume,
							VolumeOptions: &api.VolumeOptions{
								Name: "data1",
							},
						},
						{
							Name: "data2-alias",
							Type: api.VolumeTypeVolume,
							VolumeOptions: &api.VolumeOptions{
								Name: "data2",
								Driver: &mount.Driver{
									Name: "local",
								},
							},
						},
						{
							Name: "tmpfs-efa57ba8b6a1779674ac438de3af8729e2d55900b79eb929431cf9c5b0179542",
							Type: api.VolumeTypeTmpfs,
							TmpfsOptions: &mount.TmpfsOptions{
								SizeBytes: 10 * units.MiB,
							},
						},
					},
				},
				"test-caddy-config": {
					Name: "test-caddy-config",
					Mode: api.ServiceModeReplicated,
					Container: api.ContainerSpec{
						Image:      "myapp:1.2.3",
						PullPolicy: api.PullPolicyMissing,
					},
					Caddy: &api.CaddySpec{
						Config: `test-caddy-config.example.com {
  reverse_proxy {{ upstreams 80 }}
}`,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		ctx := context.Background()

		t.Run(tt.name, func(t *testing.T) {
			project, err := LoadProject(ctx, []string{filepath.Join("testdata", tt.filename)})
			require.NoError(t, err)

			for name, expectedSpec := range tt.want {
				spec, err := ServiceSpecFromCompose(project, name)
				require.NoError(t, err)

				// Due to the use of a map the order of volumes is non-deterministic.
				slices.SortFunc(spec.Volumes, func(a, b api.VolumeSpec) int {
					return strings.Compare(a.Name, b.Name)
				})

				cmpOpts := cmp.Options{cmpopts.EquateEmpty(), cmpopts.EquateComparable(netip.Addr{})}
				assert.True(t, cmp.Equal(spec, expectedSpec, cmpOpts...), cmp.Diff(spec, expectedSpec, cmpOpts...))
			}
		})
	}
}

func TestServiceSpecFromCompose_Caddy(t *testing.T) {
	tests := []struct {
		name        string
		composeYAML string
		want        *api.CaddySpec
	}{
		{
			name: "x-caddy as string",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: |
      example.com {
        reverse_proxy web:80
      }
`,
			want: &api.CaddySpec{
				Config: `example.com {
  reverse_proxy web:80
}`,
			},
		},
		{
			name: "x-caddy as string with extra spaces",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: |+

      example.com {
        reverse_proxy web:80
      }


`,
			want: &api.CaddySpec{
				Config: `example.com {
  reverse_proxy web:80
}`,
			},
		},
		{
			name: "x-caddy as object with config field",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy:
      config: |
        example.com {
          reverse_proxy web:80
        }
`,
			want: &api.CaddySpec{
				Config: `example.com {
  reverse_proxy web:80
}`,
			},
		},
		{
			name: "x-caddy as object with config field and extra spaces",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy:
      config: |+

        example.com {
          reverse_proxy web:80
        }


`,
			want: &api.CaddySpec{
				Config: `example.com {
  reverse_proxy web:80
}`,
			},
		},
		{
			name: "x-caddy with path to Caddyfile",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: testdata/Caddyfile
`,
			want: &api.CaddySpec{
				Config: `test.example.com {
  reverse_proxy test:8000
}`,
			},
		},
		{
			name: "x-caddy with empty string",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: ""
`,
			want: nil,
		},
		{
			name: "no x-caddy extension",
			composeYAML: `
services:
  web:
    image: nginx
`,
			want: nil,
		},
		{
			name: "x-caddy with empty object",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: {}
`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get current working directory for relative path resolution to testdata directory.
			wd, err := os.Getwd()
			require.NoError(t, err)

			project, err := LoadProjectFromContent(
				context.Background(),
				tt.composeYAML,
				composecli.WithWorkingDirectory(wd),
			)
			require.NoError(t, err)

			spec, err := ServiceSpecFromCompose(project, "web")
			require.NoError(t, err)

			assert.Equal(t, tt.want, spec.Caddy)
		})
	}
}

func TestServiceSpecFromCompose_GPUs(t *testing.T) {
	tests := []struct {
		name               string
		composeYAML        string
		expectedDeviceReqs []container.DeviceRequest
	}{
		{
			name: "gpus_all_shorthand",
			composeYAML: `
services:
  ai:
    image: nvidia/cuda
    gpus: all
`,
			expectedDeviceReqs: []container.DeviceRequest{
				{
					Count:        -1,
					Capabilities: [][]string{{"gpu"}},
				},
			},
		},
		{
			name: "gpus_device_ids",
			composeYAML: `
services:
  ai:
    image: nvidia/cuda
    gpus:
      - device_ids: ['0', '1']
        capabilities: [compute]
`,
			expectedDeviceReqs: []container.DeviceRequest{
				{
					DeviceIDs:    []string{"0", "1"},
					Capabilities: [][]string{{"compute", "gpu"}},
				},
			},
		},
		{
			name: "gpus_count",
			composeYAML: `
services:
  ai:
    image: nvidia/cuda
    gpus:
      - count: 2
        capabilities: [utility]
`,
			expectedDeviceReqs: []container.DeviceRequest{
				{
					Count:        2,
					Capabilities: [][]string{{"utility", "gpu"}},
				},
			},
		},
		{
			name: "gpus_driver_and_options",
			composeYAML: `
services:
  ai:
    image: nvidia/cuda
    gpus:
      - driver: nvidia
        count: 1
        capabilities: [compute, utility]
        options:
          key1: value1
          key2: value2
`,
			expectedDeviceReqs: []container.DeviceRequest{
				{
					Driver:       "nvidia",
					Count:        1,
					Capabilities: [][]string{{"compute", "utility", "gpu"}},
					Options: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
		},
		{
			name: "deploy_resources_reservations_devices",
			composeYAML: `
services:
  ai:
    image: nvidia/cuda
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
`,
			expectedDeviceReqs: []container.DeviceRequest{
				{
					Driver:       "nvidia",
					Count:        1,
					Capabilities: [][]string{{"gpu"}},
				},
			},
		},
		{
			name: "gpus_with_existing_gpu_capability",
			composeYAML: `
services:
  ai:
    image: nvidia/cuda
    gpus:
      - capabilities: [gpu, compute]
`,
			expectedDeviceReqs: []container.DeviceRequest{
				{
					// defaults to "all" when count not specified
					Count: -1,
					// gpu appended even if already present (matching Docker Compose behavior)
					Capabilities: [][]string{{"gpu", "compute", "gpu"}},
				},
			},
		},
		{
			name: "multiple_device_reservations",
			composeYAML: `
services:
  ai:
    image: nvidia/cuda
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 2
              capabilities: [gpu, compute]
            - capabilities: [tpu]
              count: 1
`,
			expectedDeviceReqs: []container.DeviceRequest{
				{
					Driver:       "nvidia",
					Count:        2,
					Capabilities: [][]string{{"gpu", "compute"}},
				},
				{
					Count:        1,
					Capabilities: [][]string{{"tpu"}},
				},
			},
		},
		{
			name: "tpu_reservation",
			composeYAML: `
services:
  ai:
    image: tensorflow/tensorflow:latest
    deploy:
      resources:
        reservations:
          devices:
            - capabilities: [tpu]
              count: 4
              driver: google
`,
			expectedDeviceReqs: []container.DeviceRequest{
				{
					Driver:       "google",
					Count:        4,
					Capabilities: [][]string{{"tpu"}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := LoadProjectFromContent(context.Background(), tt.composeYAML)
			require.NoError(t, err)

			spec, err := ServiceSpecFromCompose(project, "ai")
			require.NoError(t, err)

			assert.Equal(t, tt.expectedDeviceReqs, spec.Container.Resources.DeviceReservations,
				"DeviceReservations should match expected")
		})
	}
}

func TestServiceSpecFromCompose_XMachinesPlacement(t *testing.T) {
	tests := []struct {
		name        string
		composeYAML string
		expected    api.Placement
		expectError bool
	}{
		{
			name: "valid x-machines with string array",
			composeYAML: `
services:
  test:
    image: nginx
    x-machines: ["machine-1", "machine-2"]
`,
			expected: api.Placement{
				Machines: []string{"machine-1", "machine-2"},
			},
		},
		{
			name: "valid x-machines with single string",
			composeYAML: `
services:
  test:
    image: nginx
    x-machines: my-machine
`,
			expected: api.Placement{
				Machines: []string{"my-machine"},
			},
		},
		{
			name: "valid x-machines with single quoted string",
			composeYAML: `
services:
  test:
    image: nginx
    x-machines: "machine-1"
`,
			expected: api.Placement{
				Machines: []string{"machine-1"},
			},
		},
		{
			name: "valid x-machines with numeric string",
			composeYAML: `
services:
  test:
    image: nginx
    x-machines: "123"
`,
			expected: api.Placement{
				Machines: []string{"123"},
			},
		},
		{
			name: "valid x-machines with comma-separated string",
			composeYAML: `
services:
  test:
    image: nginx
    x-machines: "machine-1,machine-2"
`,
			expected: api.Placement{
				Machines: []string{"machine-1", "machine-2"},
			},
		},
		{
			name: "valid x-machines with comma-separated string and spaces",
			composeYAML: `
services:
  test:
    image: nginx
    x-machines: "machine-1, machine-2, machine-3"
`,
			expected: api.Placement{
				Machines: []string{"machine-1", "machine-2", "machine-3"},
			},
		},
		{
			name: "empty x-machines array",
			composeYAML: `
services:
  test:
    image: nginx
    x-machines: []
`,
			expected: api.Placement{
				Machines: []string{},
			},
		},
		{
			name: "no x-machines",
			composeYAML: `
services:
  test:
    image: nginx
`,
			expected: api.Placement{},
		},
		{
			name: "empty machine name in x-machines",
			composeYAML: `
services:
  test:
    image: nginx
    x-machines: ["machine-1", "", "machine-2"]
`,
			expectError: true,
		},
		{
			name: "empty machine name in comma-separated x-machines",
			composeYAML: `
services:
  test:
    image: nginx
    x-machines: "machine-1,,machine-2"
`,
			expectError: true,
		},
		{
			name: "x-machines with whitespace trimming",
			composeYAML: `
services:
  test:
    image: nginx
    x-machines: ["  machine-1  ", "machine-2"]
`,
			expected: api.Placement{
				Machines: []string{"machine-1", "machine-2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := LoadProjectFromContent(context.Background(), tt.composeYAML)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Convert to ServiceSpec
			spec, err := ServiceSpecFromCompose(project, "test")
			require.NoError(t, err)

			if len(tt.expected.Machines) == 0 && len(spec.Placement.Machines) == 0 {
				// Both are empty, consider them equal
				return
			}
			assert.Equal(t, tt.expected, spec.Placement)
		})
	}
}
