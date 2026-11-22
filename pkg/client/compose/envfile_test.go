//go:build unix

package compose

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func writeProjectFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestCheckEnvFileTypes(t *testing.T) {
	tests := []struct {
		name      string
		envFile   string
		setup     func(t *testing.T, path string)
		expectErr string
	}{
		{
			name:    "regular file passes",
			envFile: "MY_VAR=value\n",
		},
		{
			name: "named pipe fails with a readable error",
			setup: func(t *testing.T, path string) {
				require.NoError(t, unix.Mkfifo(path, 0o600))
			},
			expectErr: "env_file '.env.production' is not a regular file (got: fifo)",
		},
		{
			name: "directory fails",
			setup: func(t *testing.T, path string) {
				require.NoError(t, os.Mkdir(path, 0o755))
			},
			expectErr: "got: directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			envPath := filepath.Join(dir, ".env.production")

			if tt.setup != nil {
				tt.setup(t, envPath)
			} else {
				require.NoError(t, os.WriteFile(envPath, []byte(tt.envFile), 0o600))
			}

			composePath := writeProjectFile(t, dir, "compose.yaml", `services:
  web:
    image: nginx
    env_file: .env.production
`)

			err := checkEnvFileTypes(dir, []string{composePath})
			if tt.expectErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectErr)
			assert.Contains(t, err.Error(), "service 'web'")
		})
	}
}

func TestCheckEnvFileTypes_ListAndObjectForms(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, unix.Mkfifo(filepath.Join(dir, "broken.env"), 0o600))

	composePath := writeProjectFile(t, dir, "compose.yaml", `services:
  web:
    image: nginx
    env_file:
      - good.env
      - path: broken.env
        required: false
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "good.env"), []byte("A=b\n"), 0o600))

	err := checkEnvFileTypes(dir, []string{composePath})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "env_file 'broken.env' is not a regular file")
}

func TestCheckEnvFileTypes_MissingFileIsLeftToTheLoader(t *testing.T) {
	dir := t.TempDir()
	composePath := writeProjectFile(t, dir, "compose.yaml", `services:
  web:
    image: nginx
    env_file: nonexistent.env
`)

	require.NoError(t, checkEnvFileTypes(dir, []string{composePath}))
}

// TestLoadProject_FIFOEnvFile is a regression test for
// https://github.com/psviderski/uncloud/issues/331: a FIFO used as env_file must fail with a
// readable error instead of hanging the process while opening the pipe.
func TestLoadProject_FIFOEnvFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, unix.Mkfifo(filepath.Join(dir, ".env.production"), 0o600))
	composePath := writeProjectFile(t, dir, "compose.yaml", `services:
  web:
    image: nginx
    env_file: .env.production
`)

	done := make(chan error, 1)
	go func() {
		_, err := LoadProject(context.Background(), []string{composePath})
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not a regular file (got: fifo)")
	case <-time.After(10 * time.Second):
		t.Fatal("LoadProject hung on an env_file that is a named pipe")
	}
}
