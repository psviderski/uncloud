package api

import (
	"testing"

	"github.com/docker/docker/api/types/mount"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceSpec_RenderRuntimeTemplates(t *testing.T) {
	t.Parallel()

	spec := ServiceSpec{
		Container: ContainerSpec{
			Image: "busybox:latest",
			Env: EnvVars{
				"UNCHANGED": "{{.Container.Name}}",
			},
			VolumeMounts: []VolumeMount{
				{VolumeName: "config", ContainerPath: "/etc/app/{{.Container.Name}}", ReadOnly: true},
				{VolumeName: "data", ContainerPath: "/{{.Container.Name}}"},
				{VolumeName: "scratch", ContainerPath: "/tmp/{{.Container.Name}}"},
			},
			Volumes: []string{"/legacy/{{.Container.Name}}:/legacy"},
		},
		Volumes: []VolumeSpec{
			{
				Name: "config",
				Type: VolumeTypeBind,
				BindOptions: &BindOptions{
					HostPath:       "/var/lib/uncloud/{{.Container.Name}}",
					CreateHostPath: true,
					Propagation:    mount.PropagationRShared,
				},
			},
			{
				Name: "data",
				Type: VolumeTypeVolume,
				VolumeOptions: &VolumeOptions{
					Name: "data-{{.Container.Name}}",
				},
			},
			{Name: "scratch", Type: VolumeTypeTmpfs},
		},
	}
	original := spec.Clone()

	rendered, err := spec.RenderRuntimeTemplates(RuntimeTemplateContext{
		Container: RuntimeTemplateContainerContext{Name: "web-a1b2"},
	})
	require.NoError(t, err)

	assert.Equal(t, "/var/lib/uncloud/web-a1b2", rendered.Volumes[0].BindOptions.HostPath)
	assert.True(t, rendered.Volumes[0].BindOptions.CreateHostPath)
	assert.Equal(t, mount.PropagationRShared, rendered.Volumes[0].BindOptions.Propagation)
	assert.Equal(t, "data-{{.Container.Name}}", rendered.Volumes[1].VolumeOptions.Name)
	assert.Equal(t, "/etc/app/web-a1b2", rendered.Container.VolumeMounts[0].ContainerPath)
	assert.Equal(t, "/web-a1b2", rendered.Container.VolumeMounts[1].ContainerPath)
	assert.Equal(t, "/tmp/web-a1b2", rendered.Container.VolumeMounts[2].ContainerPath)
	assert.Equal(t, "{{.Container.Name}}", rendered.Container.Env["UNCHANGED"])
	assert.Equal(t, "/legacy/{{.Container.Name}}:/legacy", rendered.Container.Volumes[0])
	assert.Equal(t, original, spec)
}

func TestServiceSpec_RenderRuntimeTemplates_GoTemplateLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		runtimeTemplate string
		containerName   string
		want            string
	}{
		{
			name:            "condition variables and pipeline",
			runtimeTemplate: `/data/{{if .Container.Name}}{{$name := .Container.Name}}{{$name | printf "%s"}}{{end}}`,
			containerName:   "web-a1b2",
			want:            "/data/web-a1b2",
		},
		{
			name:            "literal delimiters",
			runtimeTemplate: `/data/{{"{{"}}.Container.Name{{"}}"}}`,
			containerName:   "web-a1b2",
			want:            "/data/{{.Container.Name}}",
		},
		{
			name:            "one pass",
			runtimeTemplate: "/data/{{.Container.Name}}",
			containerName:   "{{.Container.Name}}",
			want:            "/data/{{.Container.Name}}",
		},
		{
			name:            "static path",
			runtimeTemplate: "/data/static",
			containerName:   "web-a1b2",
			want:            "/data/static",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := serviceSpecWithBindRuntimeTemplate(tt.runtimeTemplate)
			rendered, err := spec.RenderRuntimeTemplates(RuntimeTemplateContext{
				Container: RuntimeTemplateContainerContext{Name: tt.containerName},
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, rendered.Volumes[0].BindOptions.HostPath)
		})
	}
}

func TestServiceSpec_RenderRuntimeTemplates_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		runtimeTemplate string
		containerName   string
		wantErr         string
	}{
		{
			name:            "empty container name",
			runtimeTemplate: "/data/{{.Container.Name}}",
			containerName:   "",
			wantErr:         "container name must not be empty",
		},
		{
			name:            "malformed runtime template",
			runtimeTemplate: "/data/{{.Container.Name",
			containerName:   "web-a1b2",
			wantErr:         "parse runtime template",
		},
		{
			name:            "unknown field",
			runtimeTemplate: "/data/{{.Container.ID}}",
			containerName:   "web-a1b2",
			wantErr:         "can't evaluate field ID",
		},
		{
			name:            "unregistered function",
			runtimeTemplate: `/data/{{env "HOME"}}`,
			containerName:   "web-a1b2",
			wantErr:         `function "env" not defined`,
		},
		{
			name:            "relative rendered path",
			runtimeTemplate: "{{.Container.Name}}",
			containerName:   "web-a1b2",
			wantErr:         "must be an absolute path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := serviceSpecWithBindRuntimeTemplate(tt.runtimeTemplate)
			_, err := spec.RenderRuntimeTemplates(RuntimeTemplateContext{
				Container: RuntimeTemplateContainerContext{Name: tt.containerName},
			})
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestServiceSpec_Validate_RuntimeTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		runtimeTemplate string
		wantErr         string
	}{
		{
			name:            "valid",
			runtimeTemplate: "/data/{{.Container.Name}}",
		},
		{
			name:            "malformed",
			runtimeTemplate: "/data/{{.Container.Name",
			wantErr:         "validate runtime templates",
		},
		{
			name:            "unknown field",
			runtimeTemplate: "/data/{{.Container.ID}}",
			wantErr:         "can't evaluate field ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := serviceSpecWithBindRuntimeTemplate(tt.runtimeTemplate)
			err := spec.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func serviceSpecWithBindRuntimeTemplate(hostPath string) ServiceSpec {
	return ServiceSpec{
		Name: "web",
		Container: ContainerSpec{
			Image: "busybox:latest",
			VolumeMounts: []VolumeMount{
				{VolumeName: "data", ContainerPath: "/data"},
			},
		},
		Volumes: []VolumeSpec{
			{
				Name:        "data",
				Type:        VolumeTypeBind,
				BindOptions: &BindOptions{HostPath: hostPath},
			},
		},
	}
}
