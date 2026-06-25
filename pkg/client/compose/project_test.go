package compose

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
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

// TestLoadProject_Unsupported checks that unsupported features lead to warnings or errors.
func TestLoadProject_Unsupported(t *testing.T) {
	// captureStderr runs fn while capturing stderr output and returns what was written.
	captureStderr := func(t *testing.T, fn func()) string {
		t.Helper()
		old := os.Stderr
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stderr = w
		defer func() { os.Stderr = old }()

		fn()

		w.Close()
		out, err := io.ReadAll(r)
		require.NoError(t, err)
		r.Close()
		return string(out)
	}

	tests := []struct {
		name         string
		composeYAML  string
		warnCount    int
		warnContains []string
		shouldErr    bool
	}{
		{
			name: "unsupported dns",
			composeYAML: `services:
  app:
    image: myapp:latest
    dns: 8.8.8.8
`,
			warnCount:    1,
			warnContains: []string{"dns"},
		},
		{
			name: "unsupported networks",
			composeYAML: `services:
  app:
    image: myapp:latest
    networks:
      - frontend

networks:
  frontend:
`,
			warnCount:    1,
			warnContains: []string{"networks"},
		},
		{
			name: "unsupported depends_on service_completed_successfully",
			composeYAML: `services:
  migrate:
    image: alpine
    command: ["true"]
  app:
    image: nginx
    depends_on:
      migrate:
        condition: service_completed_successfully
`,
			warnCount: 1,
			warnContains: []string{
				"service_completed_successfully",
				"pre-deploy hook",
				"https://uncloud.run/docs/guides/deployments/pre-deploy-hooks",
			},
		},
		{
			name: "multiple unsupported features",
			composeYAML: `services:
  app:
    image: myapp:latest
    dns: 8.8.8.8
    links:
      - db
  db:
    image: postgres:latest
    secrets:
      - db_password

secrets:
  db_password:
    file: ./secret.txt
`,
			warnCount:    3,
			warnContains: []string{"dns", "links", "secrets"},
		},
		{
			name: "supported networks",
			composeYAML: `services:
  app:
    image: myapp:latest
    networks:
      default: {}

  web:
    image: nginx
    networks:
      - default

networks:
  default:
`,
			warnCount: 0,
		},
		{
			name: "relative volume sources",
			composeYAML: `services:
  app:
    image: myapp:latest
    volumes:
      - ./mypath/conf:/conf:ro
      - data:/var/lib/data

volumes:
  data:
`,
			shouldErr: true,
		},
		{
			name: "home-relative volume source",
			composeYAML: `services:
  app:
    image: myapp:latest
    volumes:
      - ~/conf:/conf:ro
`,
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stderr := captureStderr(t, func() {
				_, err := LoadProjectFromContent(context.Background(), tt.composeYAML)
				if tt.shouldErr {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
			})

			if tt.warnCount == 0 {
				assert.Empty(t, stderr)
			} else {
				assert.Equal(t, tt.warnCount, strings.Count(stderr, "WARNING:"),
					"expected %d warnings, got stderr: %s", tt.warnCount, stderr)
				for _, substr := range tt.warnContains {
					assert.Contains(t, stderr, substr)
				}
			}
		})
	}
}

// TestLoadProject_Secrets covers loading and validation of all top-level secret source combinations.
func TestLoadProject_Secrets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		secret      string // YAML body under 'secrets.token'
		want        types.SecretConfig
		errContains string
	}{
		// Valid sources.
		{
			name:   "file source",
			secret: "    file: /tmp/token",
			want:   types.SecretConfig{File: "/tmp/token"},
		},
		{
			name:   "environment source",
			secret: "    environment: UNSET_SECRET_VAR",
			want:   types.SecretConfig{Environment: "UNSET_SECRET_VAR"},
		},
		{
			name:   "x-command short form expands to exec driver",
			secret: "    x-command: printf abc",
			want:   types.SecretConfig{Driver: "exec", DriverOpts: map[string]string{"command": "printf abc"}},
		},
		{
			name: "exec driver long form",
			secret: `    driver: exec
    driver_opts:
      command: printf abc`,
			want: types.SecretConfig{Driver: "exec", DriverOpts: map[string]string{"command": "printf abc"}},
		},
		// Invalid combinations.
		{
			name: "x-command with driver",
			secret: `    x-command: printf abc
    driver: exec`,
			errContains: "cannot be combined with 'driver'",
		},
		{
			name: "x-command with driver_opts",
			secret: `    x-command: printf abc
    driver_opts:
      command: printf abc`,
			errContains: "cannot be combined with 'driver'",
		},
		{
			name: "x-command with file",
			secret: `    x-command: printf abc
    file: /tmp/token`,
			errContains: "cannot be combined with 'file' or 'environment'",
		},
		{
			name: "x-command with environment",
			secret: `    x-command: printf abc
    environment: SOME_VAR`,
			errContains: "cannot be combined with 'file' or 'environment'",
		},
		{
			name:        "x-command empty",
			secret:      `    x-command: ""`,
			errContains: "must be a non-empty string",
		},
		{
			name: "unsupported driver",
			secret: `    driver: vault
    driver_opts:
      key: token`,
			errContains: "unsupported driver 'vault'",
		},
		{
			name:        "exec driver without command",
			secret:      "    driver: exec",
			errContains: "requires 'driver_opts.command'",
		},
		{
			name: "exec driver with file",
			secret: `    driver: exec
    driver_opts:
      command: printf abc
    file: /tmp/token`,
			errContains: "cannot also define 'file' or 'environment'",
		},
		{
			name:        "external not supported",
			secret:      "    external: true",
			errContains: "external secrets are not supported",
		},
		{
			name: "external with exec driver not supported",
			secret: `    driver: exec
    driver_opts:
      command: printf abc
    external: true`,
			errContains: "external secrets are not supported",
		},
		{
			name: "file and environment mutually exclusive",
			secret: `    file: /tmp/token
    environment: SOME_VAR`,
			errContains: "mutually exclusive",
		},
		{
			name:        "no source",
			secret:      "    name: token",
			errContains: "must be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			content := `
services:
  foo:
    image: foo
secrets:
  token:
` + tt.secret + "\n"
			project, err := LoadProjectFromContent(context.Background(), content)

			if tt.errContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.errContains)
				return
			}

			require.NoError(t, err)
			got := project.Secrets["token"]
			got.Name = ""
			assert.Equal(t, tt.want, got)
		})
	}
}
