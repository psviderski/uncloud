package compose

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/cli"
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
