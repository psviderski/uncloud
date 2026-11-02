package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/go-units"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validRunOptions returns the minimal options that produce a valid service spec.
func validRunOptions() runOptions {
	return runOptions{
		image:    "nginx:1.27",
		name:     "web",
		mode:     api.ServiceModeReplicated,
		pull:     api.PullPolicyMissing,
		replicas: 1,
	}
}

func TestParseLabels(t *testing.T) {
	t.Parallel()

	labels, err := parseLabels(nil)
	require.NoError(t, err)
	assert.Nil(t, labels)

	labels, err = parseLabels([]string{"tier=frontend", "empty", "list=a,b", "expr=a=b", "tier=backend"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"tier":  "backend",
		"empty": "",
		"list":  "a,b",
		"expr":  "a=b",
	}, labels)

	_, err = parseLabels([]string{"=value"})
	require.ErrorContains(t, err, "key must not be empty")

	_, err = parseLabels([]string{" \t =value"})
	require.ErrorContains(t, err, "key must not be empty")

	labels, err = parseLabels([]string{"key=value with spaces"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"key": "value with spaces"}, labels)
}

func TestPrepareServiceSpec(t *testing.T) {
	t.Parallel()

	t.Run("minimal options", func(t *testing.T) {
		t.Parallel()

		spec, err := prepareServiceSpec(validRunOptions())
		require.NoError(t, err)

		assert.Equal(t, "web", spec.Name)
		assert.Equal(t, api.ServiceModeReplicated, spec.Mode)
		assert.EqualValues(t, 1, spec.Replicas)
		assert.Equal(t, "nginx:1.27", spec.Container.Image)
		assert.Equal(t, api.PullPolicyMissing, spec.Container.PullPolicy)
		assert.Nil(t, spec.Caddy)
		assert.Nil(t, spec.Labels)
		assert.Nil(t, spec.Container.Labels)
		assert.Nil(t, spec.Container.Entrypoint)
		assert.Nil(t, spec.Container.Resources.Ulimits)
		assert.Nil(t, spec.Placement.Machines)
		assert.Empty(t, spec.Container.Env)
		assert.Empty(t, spec.Ports)
		assert.Empty(t, spec.Volumes)
		assert.Empty(t, spec.Container.VolumeMounts)
	})

	t.Run("all options", func(t *testing.T) {
		t.Parallel()

		opts := validRunOptions()
		opts.command = []string{"nginx", "-g", "daemon off;"}
		opts.env = []string{"FOO=bar", "EMPTY=", "EXPR=a=b"}
		opts.labels = []string{"tier=frontend"}
		opts.containerLabels = []string{"logging=enabled"}
		opts.machines = []string{"m1,m2", "m3"}
		opts.privileged = true
		opts.pull = api.PullPolicyAlways
		opts.publish = []string{"app.example.com:8080/https", "53:5353/udp@host"}
		opts.replicas = 3
		opts.ulimits = []string{"nofile=1024:2048", "nproc=65535"}
		opts.user = "1000:1000"
		opts.volumes = []string{"data:/var/lib/data:ro"}
		require.NoError(t, opts.cpu.Set("0.5"))
		require.NoError(t, opts.memory.Set("128m"))
		require.NoError(t, opts.memoryReservation.Set("32m"))
		require.NoError(t, opts.shmSize.Set("64m"))

		spec, err := prepareServiceSpec(opts)
		require.NoError(t, err)

		assert.Equal(t, api.ServiceSpec{
			Container: api.ContainerSpec{
				Command:    []string{"nginx", "-g", "daemon off;"},
				Env:        api.EnvVars{"FOO": "bar", "EMPTY": "", "EXPR": "a=b"},
				Image:      "nginx:1.27",
				Labels:     map[string]string{"logging": "enabled"},
				Privileged: true,
				PullPolicy: api.PullPolicyAlways,
				Resources: api.ContainerResources{
					CPU:               500 * api.MilliCore,
					Memory:            128 * units.MiB,
					MemoryReservation: 32 * units.MiB,
					SharedMemory:      64 * units.MiB,
					Ulimits: map[string]api.Ulimit{
						"nofile": {Soft: 1024, Hard: 2048},
						"nproc":  {Soft: 65535, Hard: 65535},
					},
				},
				User: "1000:1000",
				VolumeMounts: []api.VolumeMount{
					{VolumeName: "data", ContainerPath: "/var/lib/data", ReadOnly: true},
				},
			},
			Labels:    map[string]string{"tier": "frontend"},
			Mode:      api.ServiceModeReplicated,
			Name:      "web",
			Placement: api.Placement{Machines: []string{"m1", "m2", "m3"}},
			Ports: []api.PortSpec{
				{
					Hostname:      "app.example.com",
					ContainerPort: 8080,
					Protocol:      api.ProtocolHTTPS,
					Mode:          api.PortModeIngress,
				},
				{
					PublishedPort: 53,
					ContainerPort: 5353,
					Protocol:      api.ProtocolUDP,
					Mode:          api.PortModeHost,
				},
			},
			Replicas: 3,
			Volumes: []api.VolumeSpec{
				{
					Name:          "data",
					Type:          api.VolumeTypeVolume,
					VolumeOptions: &api.VolumeOptions{Name: "data"},
				},
			},
		}, spec)
	})

	t.Run("generates a name when not specified", func(t *testing.T) {
		t.Parallel()

		opts := validRunOptions()
		opts.name = ""
		opts.image = "bitnami/nginx:latest"

		spec, err := prepareServiceSpec(opts)
		require.NoError(t, err)
		assert.Regexp(t, `^nginx-[a-z0-9]{4}$`, spec.Name)
	})

	t.Run("keeps the specified name", func(t *testing.T) {
		t.Parallel()

		opts := validRunOptions()
		opts.name = "custom"

		spec, err := prepareServiceSpec(opts)
		require.NoError(t, err)
		assert.Equal(t, "custom", spec.Name)
	})

	t.Run("global mode", func(t *testing.T) {
		t.Parallel()

		opts := validRunOptions()
		opts.mode = api.ServiceModeGlobal

		spec, err := prepareServiceSpec(opts)
		require.NoError(t, err)
		assert.Equal(t, api.ServiceModeGlobal, spec.Mode)
	})

	t.Run("entrypoint", func(t *testing.T) {
		t.Parallel()

		opts := validRunOptions()
		opts.entrypoint = "/bin/sh"
		opts.entrypointChanged = true
		spec, err := prepareServiceSpec(opts)
		require.NoError(t, err)
		assert.Equal(t, []string{"/bin/sh"}, spec.Container.Entrypoint)

		// An explicitly empty entrypoint resets the one from the image.
		opts = validRunOptions()
		opts.entrypointChanged = true
		spec, err = prepareServiceSpec(opts)
		require.NoError(t, err)
		assert.Equal(t, []string{""}, spec.Container.Entrypoint)
	})

	t.Run("env without a value is omitted when not set locally", func(t *testing.T) {
		t.Parallel()

		opts := validRunOptions()
		opts.env = []string{"UNCLOUD_TEST_ENV_THAT_IS_NOT_SET"}

		spec, err := prepareServiceSpec(opts)
		require.NoError(t, err)
		assert.NotContains(t, spec.Container.Env, "UNCLOUD_TEST_ENV_THAT_IS_NOT_SET")
	})

	t.Run("bind mount", func(t *testing.T) {
		t.Parallel()

		opts := validRunOptions()
		opts.volumes = []string{"/data/uploads:/app/uploads"}

		spec, err := prepareServiceSpec(opts)
		require.NoError(t, err)

		require.Len(t, spec.Volumes, 1)
		assert.Regexp(t, `^bind-[a-z0-9]{4}$`, spec.Volumes[0].Name)
		assert.Equal(t, api.VolumeTypeBind, spec.Volumes[0].Type)
		require.NotNil(t, spec.Volumes[0].BindOptions)
		assert.Equal(t, "/data/uploads", spec.Volumes[0].BindOptions.HostPath)
		assert.True(t, spec.Volumes[0].BindOptions.CreateHostPath)

		require.Len(t, spec.Container.VolumeMounts, 1)
		assert.Equal(t, spec.Volumes[0].Name, spec.Container.VolumeMounts[0].VolumeName)
		assert.Equal(t, "/app/uploads", spec.Container.VolumeMounts[0].ContainerPath)
	})

	t.Run("named volume mounted twice creates one volume", func(t *testing.T) {
		t.Parallel()

		opts := validRunOptions()
		opts.volumes = []string{"data:/a", "data:/b"}

		spec, err := prepareServiceSpec(opts)
		require.NoError(t, err)
		assert.Len(t, spec.Volumes, 1)
		assert.Len(t, spec.Container.VolumeMounts, 2)
	})

	t.Run("caddyfile content is trimmed", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "Caddyfile")
		require.NoError(t, os.WriteFile(path, []byte("\nexample.com {\n}\n\n"), 0o600))

		opts := validRunOptions()
		opts.caddyfile = path

		spec, err := prepareServiceSpec(opts)
		require.NoError(t, err)
		require.NotNil(t, spec.Caddy)
		assert.Equal(t, "example.com {\n}", spec.Caddy.Config)
	})

	t.Run("empty caddyfile is ignored", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "Caddyfile")
		require.NoError(t, os.WriteFile(path, []byte(" \n"), 0o600))

		opts := validRunOptions()
		opts.caddyfile = path

		spec, err := prepareServiceSpec(opts)
		require.NoError(t, err)
		assert.Nil(t, spec.Caddy)
	})

	t.Run("caddyfile can be used with host ports", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "Caddyfile")
		require.NoError(t, os.WriteFile(path, []byte("example.com {\n}"), 0o600))

		opts := validRunOptions()
		opts.caddyfile = path
		opts.publish = []string{"53:5353/udp@host"}

		_, err := prepareServiceSpec(opts)
		require.NoError(t, err)
	})
}

func TestPrepareServiceSpec_LocalEnv(t *testing.T) {
	// Not parallel because t.Setenv changes the environment of the whole process.
	t.Setenv("UNCLOUD_TEST_LOCAL_ENV", "from-local")

	opts := validRunOptions()
	opts.env = []string{"UNCLOUD_TEST_LOCAL_ENV"}

	spec, err := prepareServiceSpec(opts)
	require.NoError(t, err)
	assert.Equal(t, "from-local", spec.Container.Env["UNCLOUD_TEST_LOCAL_ENV"])
}

func TestPrepareServiceSpec_Invalid(t *testing.T) {
	t.Parallel()

	caddyfile := filepath.Join(t.TempDir(), "Caddyfile")
	require.NoError(t, os.WriteFile(caddyfile, []byte("example.com {\n}"), 0o600))

	tests := []struct {
		name    string
		modify  func(opts *runOptions)
		wantErr string
	}{
		{
			name:    "invalid image",
			modify:  func(opts *runOptions) { opts.image = "INVALID IMAGE" },
			wantErr: "invalid image",
		},
		{
			name:    "invalid name",
			modify:  func(opts *runOptions) { opts.name = "Not_Valid" },
			wantErr: "invalid service name",
		},
		{
			name:    "invalid mode",
			modify:  func(opts *runOptions) { opts.mode = "unknown" },
			wantErr: "invalid replication mode: 'unknown'",
		},
		{
			name:    "invalid pull policy",
			modify:  func(opts *runOptions) { opts.pull = "sometimes" },
			wantErr: "invalid pull policy: 'sometimes'",
		},
		{
			name:    "invalid env",
			modify:  func(opts *runOptions) { opts.env = []string{"=value"} },
			wantErr: "invalid environment variable: '=value'",
		},
		{
			name:    "invalid port",
			modify:  func(opts *runOptions) { opts.publish = []string{"not-a-port"} },
			wantErr: "invalid service port 'not-a-port'",
		},
		{
			name:    "invalid ulimit",
			modify:  func(opts *runOptions) { opts.ulimits = []string{"nofile"} },
			wantErr: "invalid ulimit 'nofile'",
		},
		{
			name:    "volume without a container path",
			modify:  func(opts *runOptions) { opts.volumes = []string{"data"} },
			wantErr: "invalid volume mount 'data'",
		},
		{
			name:    "volume with a relative container path",
			modify:  func(opts *runOptions) { opts.volumes = []string{"data:relative"} },
			wantErr: "must be absolute path",
		},
		{
			name:    "volume with an unknown option",
			modify:  func(opts *runOptions) { opts.volumes = []string{"data:/data:unknown"} },
			wantErr: "invalid option: 'unknown'",
		},
		{
			name:    "missing caddyfile",
			modify:  func(opts *runOptions) { opts.caddyfile = filepath.Join(os.TempDir(), "uncloud-test-no-such-file") },
			wantErr: "read Caddyfile",
		},
		{
			name: "caddyfile with an ingress port",
			modify: func(opts *runOptions) {
				opts.caddyfile = caddyfile
				opts.publish = []string{"app.example.com:8080/https"}
			},
			wantErr: "cannot be specified simultaneously",
		},
		{
			name:    "service label without a key",
			modify:  func(opts *runOptions) { opts.labels = []string{"=value"} },
			wantErr: "key must not be empty",
		},
		{
			name:    "container label without a key",
			modify:  func(opts *runOptions) { opts.containerLabels = []string{"=value"} },
			wantErr: "key must not be empty",
		},
		{
			name:    "service label with whitespace",
			modify:  func(opts *runOptions) { opts.labels = []string{"my label=value"} },
			wantErr: "invalid service labels: label 'my label' must not contain whitespace",
		},
		{
			name:    "container label with whitespace",
			modify:  func(opts *runOptions) { opts.containerLabels = []string{"my label=value"} },
			wantErr: "invalid container labels: label 'my label' must not contain whitespace",
		},
		{
			name:    "reserved service label",
			modify:  func(opts *runOptions) { opts.labels = []string{api.LabelManaged + "=true"} },
			wantErr: "invalid service labels",
		},
		{
			name:    "reserved container label",
			modify:  func(opts *runOptions) { opts.containerLabels = []string{api.LabelManaged + "=true"} },
			wantErr: "invalid container labels",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := validRunOptions()
			tt.modify(&opts)

			_, err := prepareServiceSpec(opts)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
