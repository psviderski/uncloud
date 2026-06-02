package compose

import (
	"os"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretSpecsFromCompose(t *testing.T) {
	tests := []struct {
		name           string
		secrets        types.Secrets
		serviceSecrets []types.ServiceSecretConfig
		expectedSpecs  []api.SecretSpec
		expectedMounts []api.SecretMount
		expectError    bool
	}{
		{
			name: "project-level secret with file",
			secrets: types.Secrets{
				"app-secret": types.SecretConfig{
					File: "testdata/secret1.txt",
				},
			},
			serviceSecrets: []types.ServiceSecretConfig{
				{
					Source: "app-secret",
					Target: "/run/secrets/app-secret",
					UID:    "1000",
					GID:    "1000",
				},
			},
			expectedSpecs: []api.SecretSpec{
				{
					Name:    "app-secret",
					Content: []byte("test secret content\n"),
				},
			},
			expectedMounts: []api.SecretMount{
				{
					SecretName:    "app-secret",
					ContainerPath: "/run/secrets/app-secret",
					Uid:           "1000",
					Gid:           "1000",
				},
			},
		},
		{
			name: "secret with mode",
			secrets: types.Secrets{
				"db-password": types.SecretConfig{
					File: "./testdata/secret1.txt",
				},
			},
			serviceSecrets: []types.ServiceSecretConfig{
				{
					Source: "db-password",
					Target: "/run/secrets/db-password",
					Mode:   func() *types.FileMode { m := types.FileMode(0o400); return &m }(),
				},
			},
			expectedSpecs: []api.SecretSpec{
				{
					Name:    "db-password",
					Content: []byte("test secret content\n"),
				},
			},
			expectedMounts: []api.SecretMount{
				{
					SecretName:    "db-password",
					ContainerPath: "/run/secrets/db-password",
					Mode:          func() *os.FileMode { m := os.FileMode(0o400); return &m }(),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secretSpecs, secretMounts, err := secretSpecsFromCompose(tt.secrets, tt.serviceSecrets, ".")

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedSpecs, secretSpecs)
			assert.Equal(t, tt.expectedMounts, secretMounts)
		})
	}
}

func TestSecretSpecEquals(t *testing.T) {
	secret1 := api.SecretSpec{
		Name: "test-secret",
	}

	secret2 := api.SecretSpec{
		Name: "test-secret",
	}

	secret3 := api.SecretSpec{
		Name:    "test-secret",
		Content: []byte("some content"),
	}

	assert.True(t, secret1.Equals(secret2))
	assert.False(t, secret1.Equals(secret3))
}
