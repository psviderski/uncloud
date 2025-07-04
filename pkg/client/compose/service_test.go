package compose

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	
	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-units"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadProjectFromFile loads a compose project from a YAML file
func loadProjectFromFile(t *testing.T, filename string) *types.Project {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join("testdata", filename)
	
	options, err := cli.NewProjectOptions(
		[]string{path},
		cli.WithName(FakeProjectName),
		cli.WithOsEnv,
		cli.WithDotEnv,
	)
	require.NoError(t, err)
	
	project, err := options.LoadProject(ctx)
	require.NoError(t, err)
	
	return project
}

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
				
				assert.True(t, cmp.Equal(spec, expectedSpec, cmpopts.EquateEmpty()),
					cmp.Diff(spec, expectedSpec, cmpopts.EquateEmpty()))
			}
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
    deploy:
      placement:
        x-machines: ["machine-1", "machine-2"]
`,
			expected: api.Placement{
				Machines: []string{"machine-1", "machine-2"},
			},
		},
		{
			name: "empty x-machines array",
			composeYAML: `
services:
  test:
    image: nginx
    deploy:
      placement:
        x-machines: []
`,
			expected: api.Placement{
				Machines: []string{},
			},
		},
		{
			name: "no placement section",
			composeYAML: `
services:
  test:
    image: nginx
`,
			expected: api.Placement{},
		},
		{
			name: "placement without x-machines",
			composeYAML: `
services:
  test:
    image: nginx
    deploy:
      placement:
        constraints:
          - node.role==worker
`,
			expected: api.Placement{},
		},
		{
			name: "invalid x-machines type",
			composeYAML: `
services:
  test:
    image: nginx
    deploy:
      placement:
        x-machines: "invalid-string-not-array"
`,
			expectError: true,
		},
		{
			name: "empty machine name in x-machines",
			composeYAML: `
services:
  test:
    image: nginx
    deploy:
      placement:
        x-machines: ["machine-1", "", "machine-2"]
`,
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create project from YAML content
			configDetails := types.ConfigDetails{
				ConfigFiles: []types.ConfigFile{
					{
						Filename: "docker-compose.yml",
						Content:  []byte(tt.composeYAML),
					},
				},
			}
			
			project, err := loader.LoadWithContext(t.Context(), configDetails, func(o *loader.Options) {
				o.SetProjectName("test", true)
			})
			require.NoError(t, err)
			
			// Convert to ServiceSpec
			spec, err := ServiceSpecFromCompose(project, "test")
			
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			
			require.NoError(t, err)
			
			if len(tt.expected.Machines) == 0 && len(spec.Placement.Machines) == 0 {
				// Both are empty, consider them equal
				return
			}
			assert.Equal(t, tt.expected, spec.Placement)
		})
	}
}

func TestParseXMachines(t *testing.T) {
	tests := []struct {
		name        string
		input       interface{}
		expected    []string
		expectError bool
	}{
		{
			name:     "string slice",
			input:    []string{"machine-1", "machine-2"},
			expected: []string{"machine-1", "machine-2"},
		},
		{
			name:     "interface slice",
			input:    []interface{}{"machine-1", "machine-2"},
			expected: []string{"machine-1", "machine-2"},
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:        "invalid type - string",
			input:       "machine-1",
			expectError: true,
		},
		{
			name:        "invalid type - number",
			input:       123,
			expectError: true,
		},
		{
			name:        "interface slice with non-string",
			input:       []interface{}{"machine-1", 123},
			expectError: true,
		},
		{
			name:        "empty machine name",
			input:       []string{"machine-1", "", "machine-2"},
			expectError: true,
		},
		{
			name:     "whitespace-only machine name gets trimmed",
			input:    []string{"machine-1", "  machine-2  ", "machine-3"},
			expected: []string{"machine-1", "machine-2", "machine-3"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseXMachines(tt.input)
			
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
