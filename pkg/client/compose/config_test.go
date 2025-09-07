package compose

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigSpecsFromCompose(t *testing.T) {
	tests := []struct {
		name           string
		configs        types.Configs
		serviceConfigs []types.ServiceConfigObjConfig
		expectedSpecs  []api.ConfigSpec
		expectedMounts []api.ConfigMount
		expectError    bool
	}{
		{
			name: "project-level config with file",
			configs: types.Configs{
				"app-config": types.ConfigObjConfig{
					File: "testdata/config1.txt",
				},
			},
			serviceConfigs: []types.ServiceConfigObjConfig{
				{
					Source: "app-config",
					Target: "/app/config.json",
					UID:    "1000",
					GID:    "1000",
				},
			},
			expectedSpecs: []api.ConfigSpec{
				{
					Name:    "app-config",
					File:    "testdata/config1.txt",
					Content: "test config content\n",
				},
			},
			expectedMounts: []api.ConfigMount{
				{
					ConfigName:    "app-config",
					ContainerPath: "/app/config.json",
					UID:           "1000",
					GID:           "1000",
				},
			},
		},
		{
			name: "config with mode",
			configs: types.Configs{
				"nginx-config": types.ConfigObjConfig{
					File: "./testdata/nginx.conf",
				},
			},
			serviceConfigs: []types.ServiceConfigObjConfig{
				{
					Source: "nginx-config",
					Target: "/etc/nginx/nginx.conf",
					Mode:   func() *uint32 { m := uint32(0o644); return &m }(),
				},
			},
			expectedSpecs: []api.ConfigSpec{
				{
					Name:    "nginx-config",
					File:    "./testdata/nginx.conf",
					Content: "user nginx;\nworker_processes auto;\n",
				},
			},
			expectedMounts: []api.ConfigMount{
				{
					ConfigName:    "nginx-config",
					ContainerPath: "/etc/nginx/nginx.conf",
					Mode:          func() *uint32 { m := uint32(0o644); return &m }(),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configSpecs, configMounts, err := configSpecsFromCompose(tt.configs, tt.serviceConfigs, ".")

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedSpecs, configSpecs)
			assert.Equal(t, tt.expectedMounts, configMounts)
		})
	}
}

func TestConfigSpecEquals(t *testing.T) {
	config1 := api.ConfigSpec{
		Name: "test-config",
		File: "/path/to/config",
	}

	config2 := api.ConfigSpec{
		Name: "test-config",
		File: "/path/to/config",
	}

	config3 := api.ConfigSpec{
		Name: "test-config",
		File: "/different/path",
	}

	assert.True(t, config1.Equals(config2))
	assert.False(t, config1.Equals(config3))
}
