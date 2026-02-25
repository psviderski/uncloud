package compose

import (
	"context"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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
			name:     "full spec",
			filename: "compose-full-spec.yaml",
			want: map[string]api.ServiceSpec{
				"test": {
					Name: "test",
					Mode: api.ServiceModeReplicated,
					Container: api.ContainerSpec{
						CapAdd:     []string{"NET_ADMIN"},
						CapDrop:    []string{"ALL"},
						Command:    []string{"nginx", "updated", "command"},
						Entrypoint: []string{"/updated-docker-entrypoint.sh"},
						Env: map[string]string{
							"BOOL":  "true",
							"EMPTY": "",
							"VAR":   "value",
						},
						Healthcheck: &api.HealthcheckSpec{
							Test:          []string{"CMD", "curl", "-f", "http://localhost"},
							Interval:      1*time.Minute + 30*time.Second,
							Timeout:       10 * time.Second,
							Retries:       5,
							StartPeriod:   15 * time.Second,
							StartInterval: 2 * time.Second,
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
							Ulimits: map[string]api.Ulimit{
								"nofile": {Soft: 20000, Hard: 40000},
								"nproc":  {Soft: 65535, Hard: 65535},
							},
							Devices: []api.DeviceMapping{
								{HostPath: "/dev/ttyUSB0", ContainerPath: "/dev/ttyUSB0", CgroupPermissions: "rw"},
								{HostPath: "/dev/sda", ContainerPath: "/dev/xvda", CgroupPermissions: "rwm"},
							},
							DeviceReservations: []container.DeviceRequest{
								{Count: -1, Capabilities: [][]string{{"gpu"}}},
							},
						},
						Sysctls: map[string]string{
							"net.ipv4.ip_forward": "1",
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
								VolumeName:    "bind-53f1acbf1de61e9e608c93effca23791674e463d02bb7aaca7c625804aef1926",
								ContainerPath: "/path/in/container",
								ReadOnly:      true,
							},
							{
								VolumeName:    "data3-labeled",
								ContainerPath: "/data3",
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
					Placement: api.Placement{
						Machines: []string{"machine-1", "machine-2"},
					},
					Replicas: 3,
					UpdateConfig: api.UpdateConfig{
						Order: api.UpdateOrderStopFirst,
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "bind-53f1acbf1de61e9e608c93effca23791674e463d02bb7aaca7c625804aef1926",
							Type: api.VolumeTypeBind,
							BindOptions: &api.BindOptions{
								HostPath:       "/path/on/host",
								CreateHostPath: true,
								Propagation:    mount.Propagation("rprivate"),
							},
						},
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
							Name: "data3-labeled",
							Type: api.VolumeTypeVolume,
							VolumeOptions: &api.VolumeOptions{
								Name:    "data3-labeled",
								NoCopy:  true,
								SubPath: "app/data",
								Labels:  map[string]string{"env": "test"},
							},
						},
						{
							Name: "tmpfs-efa57ba8b6a1779674ac438de3af8729e2d55900b79eb929431cf9c5b0179542",
							Type: api.VolumeTypeTmpfs,
							TmpfsOptions: &mount.TmpfsOptions{
								SizeBytes: 10 * units.MiB,
								Mode:      os.FileMode(1770),
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

func TestServiceSpecFromCompose_VolumeDriverOpts(t *testing.T) {
	tests := []struct {
		name           string
		composeYAML    string
		expectedVolume api.VolumeSpec
	}{
		{
			name: "volume with driver_opts only (no driver specified)",
			composeYAML: `
services:
  test:
    image: nginx
    volumes:
      - nfsmount:/data/nfs
volumes:
  nfsmount:
    driver_opts:
      type: "nfs"
      o: "addr=192.168.1.100,nolock,soft"
      device: ":/mnt/share"
`,
			expectedVolume: api.VolumeSpec{
				Name: "nfsmount",
				Type: api.VolumeTypeVolume,
				VolumeOptions: &api.VolumeOptions{
					Name: "nfsmount",
					Driver: &mount.Driver{
						Options: map[string]string{
							"type":   "nfs",
							"o":      "addr=192.168.1.100,nolock,soft",
							"device": ":/mnt/share",
						},
					},
				},
			},
		},
		{
			name: "volume with driver and driver_opts",
			composeYAML: `
services:
  test:
    image: nginx
    volumes:
      - nfsmount:/data/nfs
volumes:
  nfsmount:
    driver: local
    driver_opts:
      type: "nfs"
      o: "addr=192.168.1.100,nolock,soft"
      device: ":/mnt/share"
`,
			expectedVolume: api.VolumeSpec{
				Name: "nfsmount",
				Type: api.VolumeTypeVolume,
				VolumeOptions: &api.VolumeOptions{
					Name: "nfsmount",
					Driver: &mount.Driver{
						Name: "local",
						Options: map[string]string{
							"type":   "nfs",
							"o":      "addr=192.168.1.100,nolock,soft",
							"device": ":/mnt/share",
						},
					},
				},
			},
		},
		{
			name: "external volume ignores driver_opts",
			composeYAML: `
services:
  test:
    image: nginx
    volumes:
      - extvolume:/data
volumes:
  extvolume:
    external: true
`,
			expectedVolume: api.VolumeSpec{
				Name: "extvolume",
				Type: api.VolumeTypeVolume,
				VolumeOptions: &api.VolumeOptions{
					Name: "extvolume",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := LoadProjectFromContent(context.Background(), tt.composeYAML)
			require.NoError(t, err)

			spec, err := ServiceSpecFromCompose(project, "test")
			require.NoError(t, err)

			require.Len(t, spec.Volumes, 1, "expected exactly one volume")
			actualVolume := spec.Volumes[0]

			cmpOpts := cmp.Options{cmpopts.EquateEmpty()}
			assert.True(t, cmp.Equal(actualVolume, tt.expectedVolume, cmpOpts...),
				"Volume mismatch:\n%s", cmp.Diff(actualVolume, tt.expectedVolume, cmpOpts...))
		})
	}
}

func TestServiceSpecFromCompose_Ulimits(t *testing.T) {
	tests := []struct {
		name        string
		composeYAML string
		expected    map[string]api.Ulimit
	}{
		{
			name: "single ulimit with soft and hard limits",
			composeYAML: `
services:
  db:
    image: postgres
    ulimits:
      nofile:
        soft: 20000
        hard: 40000
`,
			expected: map[string]api.Ulimit{
				"nofile": {
					Soft: 20000,
					Hard: 40000,
				},
			},
		},
		{
			name: "single ulimit with single value (soft=hard)",
			composeYAML: `
services:
  db:
    image: postgres
    ulimits:
      nproc: 65535
`,
			expected: map[string]api.Ulimit{
				"nproc": {
					Soft: 65535,
					Hard: 65535,
				},
			},
		},
		{
			name: "multiple ulimits",
			composeYAML: `
services:
  db:
    image: postgres
    ulimits:
      nofile:
        soft: 20000
        hard: 40000
      nproc: 65535
`,
			expected: map[string]api.Ulimit{
				"nofile": {
					Soft: 20000,
					Hard: 40000,
				},
				"nproc": {
					Soft: 65535,
					Hard: 65535,
				},
			},
		},
		{
			name: "empty ulimits",
			composeYAML: `
services:
  db:
    image: postgres
    ulimits: {}
`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := LoadProjectFromContent(context.Background(), tt.composeYAML)
			require.NoError(t, err)

			spec, err := ServiceSpecFromCompose(project, "db")
			require.NoError(t, err)

			assert.Equal(t, tt.expected, spec.Container.Resources.Ulimits)
		})
	}
}

func TestServiceSpecFromCompose_UpdateConfig(t *testing.T) {
	tests := []struct {
		name        string
		composeYAML string
		expected    api.UpdateConfig
		expectError bool
	}{
		{
			name: "no update_config",
			composeYAML: `
services:
  test:
    image: nginx
`,
			expected: api.UpdateConfig{},
		},
		{
			name: "update_config with stop-first order",
			composeYAML: `
services:
  test:
    image: postgres
    deploy:
      update_config:
        order: stop-first
`,
			expected: api.UpdateConfig{
				Order: api.UpdateOrderStopFirst,
			},
		},
		{
			name: "update_config with start-first order",
			composeYAML: `
services:
  test:
    image: nginx
    deploy:
      update_config:
        order: start-first
`,
			expected: api.UpdateConfig{
				Order: api.UpdateOrderStartFirst,
			},
		},
		{
			name: "update_config with invalid order",
			composeYAML: `
services:
  test:
    image: nginx
    deploy:
      update_config:
        order: invalid-order
`,
			expectError: true,
		},
		{
			name: "update_config with empty order",
			composeYAML: `
services:
  test:
    image: nginx
    deploy:
      update_config:
        parallelism: 1
`,
			expected: api.UpdateConfig{},
		},
		{
			name: "update_config with replicas and order",
			composeYAML: `
services:
  test:
    image: nginx
    deploy:
      replicas: 3
      update_config:
        order: stop-first
`,
			expected: api.UpdateConfig{
				Order: api.UpdateOrderStopFirst,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := LoadProjectFromContent(context.Background(), tt.composeYAML)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			spec, err := ServiceSpecFromCompose(project, "test")
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, tt.expected, spec.UpdateConfig)
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

func TestServiceSpecFromCompose_Devices(t *testing.T) {
	tests := []struct {
		name                 string
		composeYAML          string
		expectedDevices      []api.DeviceMapping
		expectedReservations []container.DeviceRequest
	}{
		{
			name: "simple device",
			composeYAML: `
services:
  test:
    image: nginx
    devices:
      - /dev/dri
`,
			expectedDevices: []api.DeviceMapping{
				{HostPath: "/dev/dri", ContainerPath: "/dev/dri", CgroupPermissions: "rwm"},
			},
		},
		{
			name: "device with target and permissions",
			composeYAML: `
services:
  test:
    image: nginx
    devices:
      - /dev/sda:/dev/xvda:r
`,
			expectedDevices: []api.DeviceMapping{
				{HostPath: "/dev/sda", ContainerPath: "/dev/xvda", CgroupPermissions: "r"},
			},
		},
		{
			name: "multiple devices",
			composeYAML: `
services:
  test:
    image: nginx
    devices:
      - "/dev/ttyUSB0:/dev/ttyUSB0:rw"
      - /dev/sda:/dev/xvda
      - "/dev/dri"
`,
			expectedDevices: []api.DeviceMapping{
				{HostPath: "/dev/ttyUSB0", ContainerPath: "/dev/ttyUSB0", CgroupPermissions: "rw"},
				{HostPath: "/dev/sda", ContainerPath: "/dev/xvda", CgroupPermissions: "rwm"},
				{HostPath: "/dev/dri", ContainerPath: "/dev/dri", CgroupPermissions: "rwm"},
			},
		},
		{
			name: "CDI device",
			composeYAML: `
services:
  test:
    image: nginx
    devices:
      - vendor.com/class=device1
`,
			expectedReservations: []container.DeviceRequest{
				{Driver: "cdi", DeviceIDs: []string{"vendor.com/class=device1"}},
			},
		},
		{
			name: "mixed CDI and regular devices",
			composeYAML: `
services:
  test:
    image: nginx
    devices:
      - /dev/dri
      - vendor.com/class=device1
      - nvidia.com/gpu=0
      - /dev/sda:/dev/xvda:r
`,
			expectedDevices: []api.DeviceMapping{
				{HostPath: "/dev/dri", ContainerPath: "/dev/dri", CgroupPermissions: "rwm"},
				{HostPath: "/dev/sda", ContainerPath: "/dev/xvda", CgroupPermissions: "r"},
			},
			expectedReservations: []container.DeviceRequest{
				{Driver: "cdi", DeviceIDs: []string{"vendor.com/class=device1", "nvidia.com/gpu=0"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := LoadProjectFromContent(context.Background(), tt.composeYAML)
			require.NoError(t, err)

			spec, err := ServiceSpecFromCompose(project, "test")
			require.NoError(t, err)

			assert.Equal(t, tt.expectedDevices, spec.Container.Resources.Devices)
			assert.Equal(t, tt.expectedReservations, spec.Container.Resources.DeviceReservations)
		})
	}
}
