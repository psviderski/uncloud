package caddy

import (
	"strings"
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/stretchr/testify/assert"
)

func TestResolveDeploymentOptions(t *testing.T) {
	t.Parallel()

	const (
		customImage  = "example.com/caddy:1.2.3"
		customConfig = "{\n\tdebug\n}"
	)
	machines := api.Placement{Machines: []string{"m1", "m2"}}

	// A service with a custom image, config, and machines that has the given labels.
	service := func(labels ...string) api.ServiceSpec {
		spec := api.ServiceSpec{
			Name:      client.CaddyServiceName,
			Caddy:     &api.CaddySpec{Config: customConfig},
			Container: api.ContainerSpec{Image: customImage},
			Placement: machines,
		}
		if len(labels) > 0 {
			spec.Labels = make(map[string]string, len(labels))
			for _, label := range labels {
				spec.Labels[label] = ""
			}
		}
		return spec
	}

	tests := []struct {
		name    string
		current api.ServiceSpec
		opts    deployOptions
		config  string
		want    client.CaddyDeploymentOptions
	}{
		{
			name: "no service gets the defaults",
		},
		{
			name:    "service that isn't managed gets the defaults",
			current: service(),
		},
		{
			name:    "managed service follows the default image and config and keeps machines",
			current: service(api.LabelCaddyManaged),
			want:    client.CaddyDeploymentOptions{Placement: machines},
		},
		{
			name:    "custom image is kept and config follows the default",
			current: service(api.LabelCaddyManaged, api.LabelCaddyImageCustom),
			want:    client.CaddyDeploymentOptions{Image: customImage, Placement: machines},
		},
		{
			name:    "custom config is kept and image follows the latest version",
			current: service(api.LabelCaddyManaged, api.LabelCaddyConfigCustom),
			want:    client.CaddyDeploymentOptions{Config: new(customConfig), Placement: machines},
		},
		{
			name:    "custom image and config are kept",
			current: service(api.LabelCaddyManaged, api.LabelCaddyImageCustom, api.LabelCaddyConfigCustom),
			want:    client.CaddyDeploymentOptions{Image: customImage, Config: new(customConfig), Placement: machines},
		},
		{
			name: "custom empty config is kept",
			current: api.ServiceSpec{
				Name:   client.CaddyServiceName,
				Labels: map[string]string{api.LabelCaddyManaged: "", api.LabelCaddyConfigCustom: ""},
			},
			want: client.CaddyDeploymentOptions{Config: new("")},
		},
		{
			name:    "image flag overrides the current image",
			current: service(api.LabelCaddyManaged, api.LabelCaddyImageCustom),
			opts:    deployOptions{image: "caddy:2", imageChanged: true},
			want:    client.CaddyDeploymentOptions{Image: "caddy:2", Placement: machines},
		},
		{
			name:    "empty image flag resets to the latest version",
			current: service(api.LabelCaddyManaged, api.LabelCaddyImageCustom),
			opts:    deployOptions{imageChanged: true},
			want:    client.CaddyDeploymentOptions{Placement: machines},
		},
		{
			name:    "caddyfile flag overrides the current config",
			current: service(api.LabelCaddyManaged),
			opts:    deployOptions{caddyfile: "global.Caddyfile", caddyfileChanged: true},
			config:  "{\n\tstorage redis\n}",
			want:    client.CaddyDeploymentOptions{Config: new("{\n\tstorage redis\n}"), Placement: machines},
		},
		{
			name:    "caddyfile flag with an empty file sets a custom empty config",
			current: service(api.LabelCaddyManaged),
			opts:    deployOptions{caddyfile: "empty.Caddyfile", caddyfileChanged: true},
			want:    client.CaddyDeploymentOptions{Config: new(""), Placement: machines},
		},
		{
			name:    "empty caddyfile flag resets to the default config",
			current: service(api.LabelCaddyManaged, api.LabelCaddyConfigCustom),
			opts:    deployOptions{caddyfileChanged: true},
			want:    client.CaddyDeploymentOptions{Placement: machines},
		},
		{
			name:    "machine flag overrides the current machines",
			current: service(api.LabelCaddyManaged),
			opts:    deployOptions{machines: []string{"m3,m4", "m5"}, machinesChanged: true},
			want:    client.CaddyDeploymentOptions{Placement: api.Placement{Machines: []string{"m3", "m4", "m5"}}},
		},
		{
			name:    "empty machine flag resets to all machines",
			current: service(api.LabelCaddyManaged),
			opts:    deployOptions{machines: []string{}, machinesChanged: true},
		},
		{
			name:    "flags apply to a service that isn't managed",
			current: service(),
			opts: deployOptions{
				image: "caddy:2", imageChanged: true,
				caddyfile: "global.Caddyfile", caddyfileChanged: true,
				machines: []string{"m3"}, machinesChanged: true,
			},
			config: customConfig,
			want: client.CaddyDeploymentOptions{
				Image:     "caddy:2",
				Config:    new(customConfig),
				Placement: api.Placement{Machines: []string{"m3"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveDeploymentOptions(tt.current, tt.opts, tt.config))
		})
	}
}

func TestDeploymentWarnings(t *testing.T) {
	t.Parallel()

	const (
		defaultImage = client.CaddyImage + ":0.1.3"
		customImage  = "example.com/caddy:1.2.3"
		localConfig  = "{\n\tdebug\n}"
		override     = "doesn't manage this caddy service yet"
		storage      = "certificate storage changes from"
		keepStorage  = "To keep the current storage"
		module       = "must include the Uncloud storage module"
	)
	managed := map[string]string{api.LabelCaddyManaged: ""}

	service := func(image, config string, labels map[string]string) api.ServiceSpec {
		spec := api.ServiceSpec{
			Name:      client.CaddyServiceName,
			Container: api.ContainerSpec{Image: image},
			Labels:    labels,
		}
		if config != "" {
			spec.Caddy = &api.CaddySpec{Config: config}
		}
		return spec
	}

	tests := []struct {
		name    string
		current api.ServiceSpec
		target  api.ServiceSpec
		opts    deployOptions
		want    []string
		notWant []string
	}{
		{
			name:    "new service with defaults has no warnings",
			target:  service(defaultImage, client.DefaultCaddyConfig, managed),
			notWant: []string{override, storage, module},
		},
		{
			name:    "new service with a custom image",
			target:  service(customImage, client.DefaultCaddyConfig, managed),
			want:    []string{module},
			notWant: []string{override, storage},
		},
		{
			name:    "service that isn't managed moves to cluster storage",
			current: service("caddy:2.11.4", "", nil),
			target:  service(defaultImage, client.DefaultCaddyConfig, managed),
			want:    []string{override, "from local to cluster", keepStorage},
			notWant: []string{module},
		},
		{
			name:    "service that isn't managed already uses cluster storage",
			current: service(defaultImage, client.DefaultCaddyConfig, nil),
			target:  service(defaultImage, client.DefaultCaddyConfig, managed),
			want:    []string{override},
			notWant: []string{storage, module},
		},
		{
			name:    "managed service without changes has no warnings",
			current: service(defaultImage, client.DefaultCaddyConfig, managed),
			target:  service(defaultImage, client.DefaultCaddyConfig, managed),
			notWant: []string{override, storage, module},
		},
		{
			name:    "managed service upgrades the default image",
			current: service(client.CaddyImage+":0.1.2", client.DefaultCaddyConfig, managed),
			target:  service(defaultImage, client.DefaultCaddyConfig, managed),
			notWant: []string{override, storage, module},
		},
		{
			name:    "custom caddyfile moves to local storage",
			current: service(defaultImage, client.DefaultCaddyConfig, managed),
			target:  service(defaultImage, localConfig, managed),
			opts:    deployOptions{caddyfile: "global.Caddyfile", caddyfileChanged: true},
			want:    []string{"from cluster to local"},
			notWant: []string{override, keepStorage, module},
		},
		{
			name:    "other storage module is named",
			current: service(defaultImage, "{\n\tstorage redis\n}", managed),
			target:  service(defaultImage, client.DefaultCaddyConfig, managed),
			want:    []string{"from 'redis' to cluster", keepStorage},
		},
		{
			name:    "custom image is unchanged with the same config",
			current: service(customImage, client.DefaultCaddyConfig, managed),
			target:  service(customImage, client.DefaultCaddyConfig, managed),
			notWant: []string{override, storage, module},
		},
		{
			name:    "custom image receives a changed default config",
			current: service(customImage, "{\n\tstorage uncloud\n\tdebug\n}", managed),
			target:  service(customImage, client.DefaultCaddyConfig, managed),
			want:    []string{module},
			notWant: []string{override, storage},
		},
		{
			name:    "custom image with local storage",
			current: service(defaultImage, localConfig, managed),
			target:  service(customImage, localConfig, managed),
			notWant: []string{override, storage, module},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := strings.Join(deploymentWarnings(tt.current, tt.target, tt.opts), "\n")
			for _, want := range tt.want {
				assert.Contains(t, warnings, want)
			}
			for _, notWant := range tt.notWant {
				assert.NotContains(t, warnings, notWant)
			}
		})
	}
}
