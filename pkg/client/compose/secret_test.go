package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadProject loads a compose project from content with a working directory that exists for the duration
// of the test. LoadProjectFromContent removes its own temporary directory before returning, so secret
// commands (which run in the project working directory) need a valid one.
func loadProject(t *testing.T, content string) *types.Project {
	t.Helper()

	project, err := LoadProjectFromContent(context.Background(), content)
	require.NoError(t, err)
	project.WorkingDir = t.TempDir()

	return project
}

// resolvedEnv loads a compose project from content, resolves its secrets, and returns the resolved
// environment of the given service.
func resolvedEnv(t *testing.T, content, service string) types.MappingWithEquals {
	t.Helper()

	project := loadProject(t, content)
	require.NoError(t, ResolveSecrets(context.Background(), project))

	return project.Services[service].Environment
}

// env builds a MappingWithEquals from a plain map for comparing against resolved service environments.
func env(vars map[string]string) types.MappingWithEquals {
	return types.Mapping(vars).ToMappingWithEquals()
}

func TestResolveSecrets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    types.MappingWithEquals
	}{
		{
			name: "short form x-command",
			content: `
services:
  foo:
    image: foo
    environment:
      TOKEN: secret://token
secrets:
  token:
    x-command: printf 'topsecret'
`,
			want: env(map[string]string{"TOKEN": "topsecret"}),
		},
		{
			name: "long form driver exec",
			content: `
services:
  foo:
    image: foo
    environment:
      TOKEN: secret://token
secrets:
  token:
    driver: exec
    driver_opts:
      command: printf 'topsecret'
`,
			want: env(map[string]string{"TOKEN": "topsecret"}),
		},
		{
			name: "command output trailing newline trimmed but surrounding space kept",
			content: `
services:
  foo:
    image: foo
    environment:
      TOKEN: secret://token
secrets:
  token:
    x-command: printf '  spaced value  \n'
`,
			want: env(map[string]string{"TOKEN": "  spaced value  "}),
		},
		{
			name: "empty command output is allowed",
			content: `
services:
  foo:
    image: foo
    environment:
      TOKEN: secret://token
secrets:
  token:
    x-command: printf ''
`,
			want: env(map[string]string{"TOKEN": ""}),
		},
		{
			name: "non-secret env value left untouched",
			content: `
services:
  foo:
    image: foo
    environment:
      PLAIN: hello
      TOKEN: secret://token
secrets:
  token:
    x-command: printf 'abc'
`,
			want: env(map[string]string{"PLAIN": "hello", "TOKEN": "abc"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, resolvedEnv(t, tt.content, "foo"))
		})
	}
}

func TestResolveSecrets_FileSource(t *testing.T) {
	t.Parallel()

	secretFile := filepath.Join(t.TempDir(), "token.txt")
	// File content is used verbatim, including the trailing newline.
	require.NoError(t, os.WriteFile(secretFile, []byte("file-secret\n"), 0o600))

	// Use an absolute path so it survives LoadProjectFromContent removing its working directory.
	content := fmt.Sprintf(`
services:
  foo:
    image: foo
    environment:
      TOKEN: secret://token
secrets:
  token:
    file: %s
`, secretFile)

	assert.Equal(t, env(map[string]string{"TOKEN": "file-secret\n"}), resolvedEnv(t, content, "foo"))
}

func TestResolveSecrets_CommandUsesProjectEnvironment(t *testing.T) {
	t.Parallel()

	// A variable defined only in a .env file (not in the process environment) must be available to the
	// secret command, proving the resolved project environment is passed to it, not just os.Environ().
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("DOTENV_ONLY=from-dotenv\n"), 0o600))
	// '$$' escapes the '$' so Compose doesn't interpolate it; the explicit shell expands it at run time.
	composeYAML := `
services:
  foo:
    image: foo
    environment:
      TOKEN: secret://token
secrets:
  token:
    x-command: sh -c 'printf %s "$$DOTENV_ONLY"'
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(composeYAML), 0o644))

	project, err := LoadProject(context.Background(), []string{filepath.Join(dir, "compose.yaml")})
	require.NoError(t, err)
	require.NoError(t, ResolveSecrets(context.Background(), project))

	assert.Equal(t, env(map[string]string{"TOKEN": "from-dotenv"}), project.Services["foo"].Environment)
}

func TestResolveSecrets_EnvironmentSource(t *testing.T) {
	// Surrounding whitespace verifies the environment value is used verbatim, not trimmed.
	t.Setenv("MY_SECRET_VAR", "  from-env  ")

	content := `
services:
  foo:
    image: foo
    environment:
      TOKEN: secret://token
secrets:
  token:
    environment: MY_SECRET_VAR
`
	assert.Equal(t, env(map[string]string{"TOKEN": "  from-env  "}), resolvedEnv(t, content, "foo"))
}

func TestResolveSecrets_EnvironmentSourceEmpty(t *testing.T) {
	// A variable that is set but empty is a valid value, distinct from an unset variable.
	t.Setenv("MY_EMPTY_VAR", "")

	content := `
services:
  foo:
    image: foo
    environment:
      TOKEN: secret://token
secrets:
  token:
    environment: MY_EMPTY_VAR
`
	assert.Equal(t, env(map[string]string{"TOKEN": ""}), resolvedEnv(t, content, "foo"))
}

func TestResolveSecrets_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		errContains string
	}{
		{
			name: "undefined secret",
			content: `
services:
  foo:
    image: foo
    environment:
      TOKEN: secret://missing
`,
			errContains: "is not defined in the top-level 'secrets' section",
		},
		{
			name: "command fails with stderr",
			content: `
services:
  foo:
    image: foo
    environment:
      TOKEN: secret://token
secrets:
  token:
    x-command: sh -c 'echo boom >&2; exit 3'
`,
			errContains: "boom",
		},
		{
			name: "environment variable not set",
			content: `
services:
  foo:
    image: foo
    environment:
      TOKEN: secret://token
secrets:
  token:
    environment: DEFINITELY_UNSET_VAR_XYZ
`,
			errContains: "environment variable 'DEFINITELY_UNSET_VAR_XYZ' is not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			project := loadProject(t, tt.content)
			err := ResolveSecrets(context.Background(), project)
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.errContains)
		})
	}
}

// TestResolveSecrets_RunsOnce verifies a secret's command runs only once even when referenced by
// multiple services.
func TestResolveSecrets_RunsOnce(t *testing.T) {
	t.Parallel()

	counter := filepath.Join(t.TempDir(), "runs")
	content := fmt.Sprintf(`
services:
  foo:
    image: foo
    environment:
      TOKEN: secret://token
  bar:
    image: bar
    environment:
      TOKEN: secret://token
secrets:
  token:
    x-command: "sh -c 'echo run >> %s; printf abc'"
`, counter)

	project := loadProject(t, content)
	require.NoError(t, ResolveSecrets(context.Background(), project))

	foo, err := ServiceSpecFromCompose(project, "foo")
	require.NoError(t, err)
	assert.Equal(t, "abc", foo.Container.Env["TOKEN"])
	bar, err := ServiceSpecFromCompose(project, "bar")
	require.NoError(t, err)
	assert.Equal(t, "abc", bar.Container.Env["TOKEN"])

	runs, err := os.ReadFile(counter)
	require.NoError(t, err)
	assert.Equal(t, "run\n", string(runs), "command should run exactly once for both services")
}
