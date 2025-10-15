package compose

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadProject_EnvFileInterpolation tests that environment variables from .env files
// are properly loaded and available for interpolation in compose files.
func TestLoadProject_EnvFileInterpolation(t *testing.T) {
	tests := []struct {
		name        string
		composeYAML string
		envFile     string
		verify      func(t *testing.T, projectDir string)
	}{
		// This is a regression test for https://github.com/psviderski/uncloud/issues/144.
		{
			name: "simple variable interpolation",
			envFile: `KEY1=value1
KEY2=value2
`,
			composeYAML: `services:
  web:
    image: nginx
    command: ["echo", "${KEY1}", "${KEY2}"]
`,
			verify: func(t *testing.T, projectDir string) {
				project, err := LoadProject(context.Background(), []string{filepath.Join(projectDir, "compose.yaml")})
				require.NoError(t, err)
				require.NotNil(t, project)
				require.Len(t, project.Services, 1)

				webService := project.Services["web"]
				require.NotNil(t, webService)
				require.Len(t, webService.Command, 3)
				assert.Equal(t, "echo", webService.Command[0])
				assert.Equal(t, "value1", webService.Command[1], "KEY1 should be interpolated from .env file")
				assert.Equal(t, "value2", webService.Command[2], "KEY2 should be interpolated from .env file")
			},
		},
		{
			name: "environment variable interpolation in service environment",
			envFile: `DATABASE_URL=postgres://localhost:5432/mydb
REDIS_URL=redis://localhost:6379
`,
			composeYAML: `services:
  app:
    image: myapp:latest
    environment:
      - DB_CONNECTION=${DATABASE_URL}
      - CACHE_URL=${REDIS_URL}
`,
			verify: func(t *testing.T, projectDir string) {
				project, err := LoadProject(context.Background(), []string{filepath.Join(projectDir, "compose.yaml")})
				require.NoError(t, err)
				require.NotNil(t, project)
				require.Len(t, project.Services, 1)

				appService := project.Services["app"]
				require.NotNil(t, appService)
				require.NotNil(t, appService.Environment)

				assert.Equal(t, "postgres://localhost:5432/mydb", *appService.Environment["DB_CONNECTION"],
					"DATABASE_URL should be interpolated from .env file")
				assert.Equal(t, "redis://localhost:6379", *appService.Environment["CACHE_URL"],
					"REDIS_URL should be interpolated from .env file")
			},
		},
		{
			name: "variable with default value when not in env file",
			envFile: `EXISTING_VAR=exists
`,
			composeYAML: `services:
  test:
    image: busybox
    environment:
      - VAR1=${EXISTING_VAR}
      - VAR2=${MISSING_VAR:-default_value}
`,
			verify: func(t *testing.T, projectDir string) {
				project, err := LoadProject(context.Background(), []string{filepath.Join(projectDir, "compose.yaml")})
				require.NoError(t, err)
				require.NotNil(t, project)

				testService := project.Services["test"]
				require.NotNil(t, testService)

				assert.Equal(t, "exists", *testService.Environment["VAR1"],
					"EXISTING_VAR should be interpolated from .env file")
				assert.Equal(t, "default_value", *testService.Environment["VAR2"],
					"MISSING_VAR should use default value")
			},
		},
		{
			name: "os environment overrides .env file",
			envFile: `MY_VAR=from_env_file
`,
			composeYAML: `services:
  override:
    image: alpine
    environment:
      - TEST_VAR=${MY_VAR}
`,
			verify: func(t *testing.T, projectDir string) {
				t.Setenv("MY_VAR", "from_os_env")

				project, err := LoadProject(context.Background(), []string{filepath.Join(projectDir, "compose.yaml")})
				require.NoError(t, err)
				require.NotNil(t, project)

				overrideService := project.Services["override"]
				require.NotNil(t, overrideService)

				// OS environment should win over .env file (WithOsEnv comes before WithDotEnv).
				assert.Equal(t, "from_os_env", *overrideService.Environment["TEST_VAR"],
					"OS environment should override .env file")
			},
		},
		{
			name:    "no .env file - missing variables get empty string",
			envFile: "",
			composeYAML: `services:
  missing:
    image: ubuntu
    command: ["echo", "${UNDEFINED_VAR}"]
`,
			verify: func(t *testing.T, projectDir string) {
				os.Remove(filepath.Join(projectDir, ".env"))

				project, err := LoadProject(context.Background(), []string{filepath.Join(projectDir, "compose.yaml")})
				require.NoError(t, err)
				require.NotNil(t, project)

				missingService := project.Services["missing"]
				require.NotNil(t, missingService)
				require.Len(t, missingService.Command, 2)
				assert.Equal(t, "", missingService.Command[1], "undefined variable should be empty string")
			},
		},
		{
			name:    "COMPOSE_DISABLE_ENV_FILE disables .env loading",
			envFile: `MY_VAR=from_env_file`,
			composeYAML: `services:
  web:
    image: nginx
    command: ["echo", "${MY_VAR}"]
`,
			verify: func(t *testing.T, projectDir string) {
				t.Setenv("COMPOSE_DISABLE_ENV_FILE", "true")

				project, err := LoadProject(context.Background(), []string{filepath.Join(projectDir, "compose.yaml")})
				require.NoError(t, err)
				require.NotNil(t, project)

				webService := project.Services["web"]
				require.NotNil(t, webService)
				require.Len(t, webService.Command, 2)

				assert.Equal(t, "", webService.Command[1], "variable should be empty when .env file is disabled")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			composeFile := filepath.Join(tempDir, "compose.yaml")
			err := os.WriteFile(composeFile, []byte(tt.composeYAML), 0o644)
			require.NoError(t, err)

			// Write .env file if provided.
			if tt.envFile != "" {
				envFile := filepath.Join(tempDir, ".env")
				err = os.WriteFile(envFile, []byte(tt.envFile), 0o644)
				require.NoError(t, err)
			}

			tt.verify(t, tempDir)
		})
	}
}
