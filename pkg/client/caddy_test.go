package client

import (
	"context"
	"testing"

	"github.com/distribution/reference"
	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatestCaddyImage(t *testing.T) {
	t.Parallel()

	image, err := latestCaddyImage(context.Background())
	require.NoError(t, err)

	assert.Regexp(t, `^ghcr\.io/unlabs-dev/caddy-uncloud:0\.\d+\.\d+$`, reference.FamiliarString(image))
}

func TestLatestCaddyTag(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		tags []string
		want string
	}{
		{name: "empty", want: "latest"},
		{name: "semantic ordering", tags: []string{"0.1.9", "0.1.11", "0.1.10", "0.1.2"}, want: "0.1.11"},
		{name: "newest minor version", tags: []string{"0.1.3", "0.2.0", "0.0.9"}, want: "0.2.0"},
		{name: "ignore newer major versions", tags: []string{"1.0.0", "0.1.3", "2.3.4", "1.0.1"}, want: "0.1.3"},
		{name: "ignore variants", tags: []string{
			"latest", "0.1.4-rc.1", "0.1.4-alpine", "0.1", "v0.1.4", "1.0.0-rc.1", "0.1.3",
		}, want: "0.1.3"},
		{name: "no stable version", tags: []string{"builder", "0.1.4-beta.1"}, want: "latest"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, latestCaddyTag(tt.tags))
		})
	}
}

func TestDefaultCaddyConfig_UsesUncloudStorage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, CaddyStorageUncloud, CaddyStorageModule(DefaultCaddyConfig))
}

func TestCaddyStorageModule(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		config string
		want   string
	}{
		{name: "empty config", want: CaddyStorageFileSystem},
		{name: "no storage option", config: "{\n\tdebug\n}", want: CaddyStorageFileSystem},
		{name: "uncloud", config: "{\n\tstorage uncloud\n}", want: CaddyStorageUncloud},
		{name: "uncloud with options", config: "{\n\tstorage uncloud {\n\t\tfoo bar\n\t}\n}", want: CaddyStorageUncloud},
		{name: "spaces around", config: "{\n    storage   uncloud  \n}", want: CaddyStorageUncloud},
		{name: "other module", config: "{\n\tstorage redis\n}", want: "redis"},
		{name: "explicit file system", config: "{\n\tstorage file_system /data\n}", want: CaddyStorageFileSystem},
		{name: "commented out", config: "{\n\t# storage uncloud\n}", want: CaddyStorageFileSystem},
		{name: "similar option", config: "{\n\tstorage_clean_interval 24h\n}", want: CaddyStorageFileSystem},
		{name: "no module on the same line", config: "{\n\tstorage\n\tdebug\n}", want: CaddyStorageFileSystem},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CaddyStorageModule(tt.config))
		})
	}
}

func TestIsDefaultCaddyImage(t *testing.T) {
	t.Parallel()

	assert.True(t, IsDefaultCaddyImage("ghcr.io/unlabs-dev/caddy-uncloud"))
	assert.True(t, IsDefaultCaddyImage("ghcr.io/unlabs-dev/caddy-uncloud:0.1.3"))
	assert.False(t, IsDefaultCaddyImage("caddy:2.11.4"))
	assert.False(t, IsDefaultCaddyImage("example.com/unlabs-dev/caddy-uncloud:0.1.3"))
	assert.False(t, IsDefaultCaddyImage(""))
}

func TestIsNewerCaddyImage(t *testing.T) {
	t.Parallel()

	const repo = "ghcr.io/unlabs-dev/caddy-uncloud"
	for _, tt := range []struct {
		name  string
		image string
		other string
		want  bool
	}{
		{name: "newer major version", image: repo + ":1.0.0", other: repo + ":0.1.3", want: true},
		{name: "newer minor version", image: repo + ":0.2.0", other: repo + ":0.1.3", want: true},
		{name: "same", image: repo + ":0.1.3", other: repo + ":0.1.3"},
		{name: "older", image: repo + ":0.1.2", other: repo + ":0.1.3"},
		{name: "different repo", image: "example.com/custom:1.0.0", other: repo + ":0.1.3"},
		{name: "official image", image: "caddy:2.11.4", other: repo + ":0.1.3"},
		{name: "tag without version", image: repo + ":latest", other: repo + ":0.1.3"},
		{name: "no tag", image: repo, other: repo + ":0.1.3"},
		{name: "fallback to latest tag", image: repo + ":1.0.0", other: repo + ":latest"},
		{name: "no current image", other: repo + ":0.1.3"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsNewerCaddyImage(tt.image, tt.other))
		})
	}
}

func TestCaddyClientNewDeployment(t *testing.T) {
	t.Parallel()

	const customImage = "example.com/caddy:1.2.3"
	for _, tt := range []struct {
		name       string
		opts       CaddyDeploymentOptions
		wantConfig string
		wantLabels map[string]string
	}{
		{
			name:       "defaults",
			wantConfig: DefaultCaddyConfig,
			wantLabels: map[string]string{api.LabelCaddyManaged: ""},
		},
		{
			name:       "custom image",
			opts:       CaddyDeploymentOptions{Image: customImage},
			wantConfig: DefaultCaddyConfig,
			wantLabels: map[string]string{api.LabelCaddyManaged: "", api.LabelCaddyImageCustom: ""},
		},
		{
			name:       "custom config",
			opts:       CaddyDeploymentOptions{Config: new("{\n\tdebug\n}")},
			wantConfig: "{\n\tdebug\n}",
			wantLabels: map[string]string{api.LabelCaddyManaged: "", api.LabelCaddyConfigCustom: ""},
		},
		{
			name:       "custom empty config",
			opts:       CaddyDeploymentOptions{Config: new("")},
			wantLabels: map[string]string{api.LabelCaddyManaged: "", api.LabelCaddyConfigCustom: ""},
		},
		{
			name:       "custom config matching the default is still custom",
			opts:       CaddyDeploymentOptions{Config: new(DefaultCaddyConfig)},
			wantConfig: DefaultCaddyConfig,
			wantLabels: map[string]string{api.LabelCaddyManaged: "", api.LabelCaddyConfigCustom: ""},
		},
		{
			name: "custom image and config",
			opts: CaddyDeploymentOptions{
				Image:  customImage,
				Config: new("{\n\tdebug\n}"),
			},
			wantConfig: "{\n\tdebug\n}",
			wantLabels: map[string]string{
				api.LabelCaddyManaged:      "",
				api.LabelCaddyImageCustom:  "",
				api.LabelCaddyConfigCustom: "",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := &CaddyClient{client: &Client{}}
			d, err := c.NewDeployment(context.Background(), tt.opts)
			require.NoError(t, err)

			assert.Equal(t, tt.wantConfig, d.Spec.CaddyConfig())
			assert.Equal(t, tt.wantLabels, d.Spec.Labels)
		})
	}

	t.Run("official image needs a config without cluster storage", func(t *testing.T) {
		c := &CaddyClient{client: &Client{}}
		_, err := c.NewDeployment(context.Background(), CaddyDeploymentOptions{Image: "caddy:2.11.4"})
		require.ErrorContains(t, err, "doesn't support 'storage uncloud'")

		_, err = c.NewDeployment(context.Background(), CaddyDeploymentOptions{Image: "caddy:2.11.4", Config: new("")})
		require.NoError(t, err)
	})
}

func TestValidateCaddyImageStorage(t *testing.T) {
	t.Parallel()

	const storageConfig = "{\n\tstorage uncloud\n}"
	for _, tt := range []struct {
		name    string
		image   string
		config  string
		wantErr bool
	}{
		{name: "official image with uncloud storage", image: "caddy:2.11.4", config: storageConfig, wantErr: true},
		{name: "official image with registry", image: "docker.io/library/caddy:2", config: storageConfig, wantErr: true},
		{name: "official image without config", image: "caddy:2.11.4"},
		{name: "official image with other config", image: "caddy:2.11.4", config: "{\n\tdebug\n}"},
		{name: "default image with uncloud storage", image: CaddyImage + ":0.1.3", config: storageConfig},
		{name: "custom image with uncloud storage", image: "example.com/caddy:1", config: storageConfig},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCaddyImageStorage(tt.image, tt.config)
			if tt.wantErr {
				require.ErrorContains(t, err, "doesn't support 'storage uncloud'")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestOutdatedCaddyDaemons(t *testing.T) {
	t.Parallel()

	machine := func(name, version string) *pb.MachineMember {
		return &pb.MachineMember{Machine: &pb.MachineInfo{Name: name, DaemonVersion: version}}
	}

	assert.Equal(t, []string{"old (0.21.0)", "older (0.20.0)", "unsupported (0.9.3)"},
		outdatedCaddyDaemons(api.MachineMembersList{
			machine("old", "0.21.0"),
			machine("older", "0.20.0"),
			machine("unsupported", "0.9.3"),
			machine("no-version", ""),
			machine("required", "0.22.0"),
			machine("newer", "0.23.1"),
			machine("major", "1.0.0"),
			machine("nightly", "0.22.0-nightly-abc1234"),
			machine("old-dev-build", "v0.21.0-5-g6c7ddeb-dirty"),
			machine("local-build", "999.0.0-dev"),
			machine("not-semver", "6c7ddeb"),
		}))
	assert.Empty(t, outdatedCaddyDaemons(api.MachineMembersList{machine("required", "0.22.0")}))
	assert.Empty(t, outdatedCaddyDaemons(nil))
}
