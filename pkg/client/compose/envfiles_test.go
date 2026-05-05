package compose

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const windowsGOOS = "windows"

// writeCompose writes a compose.yaml referencing the given env_file fragment
// and returns the absolute compose file path.
func writeCompose(t *testing.T, dir, envFileFragment string) string {
	t.Helper()
	yaml := "services:\n  web:\n    image: nginx\n    env_file: " + envFileFragment + "\n"
	p := filepath.Join(dir, "compose.yaml")
	require.NoError(t, os.WriteFile(p, []byte(yaml), 0o644))
	return p
}

// loadWithTimeout calls LoadProject with a hard timeout and returns whether it
// completed in time and the resulting error. Used to pin "no hang" on fixtures
// that previously blocked indefinitely on open(2).
func loadWithTimeout(t *testing.T, composePath string) (completed bool, err error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, e := LoadProject(ctx, []string{composePath})
		done <- e
	}()
	select {
	case err = <-done:
		return true, err
	case <-time.After(3 * time.Second):
		cancel()
		return false, nil
	}
}

// TestValidateEnvFiles_FIFOIsRejected pins issue #331: a Compose service whose
// env_file is a named pipe used to hang `uc deploy` indefinitely because
// compose-go's open(2) blocks until a writer connects. We now reject the FIFO
// up front with a clear error.
func TestValidateEnvFiles_FIFOIsRejected(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("FIFOs are not first-class on Windows")
	}
	dir := t.TempDir()
	fifo := filepath.Join(dir, ".env.production")
	require.NoError(t, syscall.Mkfifo(fifo, 0o644))

	composePath := writeCompose(t, dir, ".env.production")

	completed, err := loadWithTimeout(t, composePath)
	require.True(t, completed, "LoadProject must not hang on a FIFO env_file")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "env_file")
	assert.Contains(t, err.Error(), "named pipe (FIFO)")
	assert.Contains(t, err.Error(), "web", "error must name the offending service")
}

// TestValidateEnvFiles_RegularFilePasses asserts the validator does not break
// the happy path: a normal env_file still loads and its variables are visible
// to interpolation.
func TestValidateEnvFiles_RegularFilePasses(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env.production")
	require.NoError(t, os.WriteFile(envPath, []byte("REGULAR_KEY=ok\n"), 0o644))

	composePath := writeCompose(t, dir, ".env.production")

	project, err := LoadProject(context.Background(), []string{composePath})
	require.NoError(t, err)
	require.NotNil(t, project)
	require.Len(t, project.Services, 1)
	web := project.Services["web"]
	require.NotNil(t, web.Environment["REGULAR_KEY"])
	assert.Equal(t, "ok", *web.Environment["REGULAR_KEY"])
}

// TestValidateEnvFiles_MissingFileDeferredToComposeGo asserts that we do not
// pre-empt compose-go's missing-file diagnostics. A non-existent env_file
// should still produce compose-go's canonical "no such file" error (or be
// silently skipped when required: false), not our "non-regular file" error.
func TestValidateEnvFiles_MissingFileDeferredToComposeGo(t *testing.T) {
	dir := t.TempDir()
	composePath := writeCompose(t, dir, ".env.absent")

	_, err := LoadProject(context.Background(), []string{composePath})
	require.Error(t, err)
	// Our pre-flight should not fire for missing files. compose-go's own
	// error path covers them, so we should never see our new label here.
	assert.NotContains(t, err.Error(), "is not a regular file")
}

// TestValidateEnvFiles_DirectoryIsRejected asserts that a directory in place
// of an env_file is rejected with a clear error rather than letting compose-go
// fail later with a confusing read error.
func TestValidateEnvFiles_DirectoryIsRejected(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, ".env.production")
	require.NoError(t, os.Mkdir(subDir, 0o755))

	composePath := writeCompose(t, dir, ".env.production")

	_, err := LoadProject(context.Background(), []string{composePath})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "env_file")
	assert.Contains(t, err.Error(), "directory")
}

// TestValidateEnvFiles_SocketIsRejected covers another common non-regular case:
// a Unix domain socket in place of an env_file. open(2) on a socket returns
// ENXIO on Linux but the failure mode is opaque for users, so reject early.
func TestValidateEnvFiles_SocketIsRejected(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("Unix sockets are not first-class on Windows")
	}
	// macOS caps sun_path at 104 chars, and the default t.TempDir() under
	// /var/folders/.../001/ already exceeds that on its own. Bind under a
	// short /tmp path and clean up explicitly.
	dir, err := os.MkdirTemp("/tmp", "uc-sock-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	sockPath := filepath.Join(dir, "s")
	l, err := net.Listen("unix", sockPath)
	require.NoError(t, err)
	defer l.Close()

	yaml := "services:\n  web:\n    image: nginx\n    env_file: s\n"
	composePath := filepath.Join(dir, "compose.yaml")
	require.NoError(t, os.WriteFile(composePath, []byte(yaml), 0o644))

	_, err = LoadProject(context.Background(), []string{composePath})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "env_file")
	assert.Contains(t, err.Error(), "socket")
}

// TestValidateEnvFiles_ListSyntax asserts the FIFO check triggers regardless
// of which of env_file's three syntactic forms the user chose. Compose lets
// you write `env_file:` as a string, a list of strings, or a list of objects
// with `path`/`required`. compose-go transforms them into a single canonical
// shape internally; this guards that the validator agrees.
func TestValidateEnvFiles_ListSyntax(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("FIFOs are not first-class on Windows")
	}
	tests := []struct {
		name     string
		fragment string
	}{
		{name: "list of strings", fragment: "[\".env.good\", \".env.bad\"]"},
		{name: "list of objects", fragment: "[{path: .env.good, required: true}, {path: .env.bad, required: false}]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.good"), []byte("X=1\n"), 0o644))
			require.NoError(t, syscall.Mkfifo(filepath.Join(dir, ".env.bad"), 0o644))

			composePath := writeCompose(t, dir, tt.fragment)

			completed, err := loadWithTimeout(t, composePath)
			require.True(t, completed, "LoadProject must not hang on a FIFO env_file")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "named pipe (FIFO)")
			assert.Contains(t, err.Error(), ".env.bad")
		})
	}
}

// TestEnvFilePaths_ExtractsAllShapes covers the path extractor in isolation,
// pinning that each of the three Compose syntactic forms produces the right
// list. Empty / blank entries are dropped so they cannot stat the project
// working dir.
func TestEnvFilePaths_ExtractsAllShapes(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want []string
	}{
		{name: "nil", in: nil, want: nil},
		{name: "empty string", in: "", want: nil},
		{name: "single string", in: ".env", want: []string{".env"}},
		{name: "list of strings", in: []any{".env.a", ".env.b"}, want: []string{".env.a", ".env.b"}},
		{name: "list with empty string skipped", in: []any{".env.a", ""}, want: []string{".env.a"}},
		{
			name: "list of objects",
			in: []any{
				map[string]any{"path": ".env.a", "required": true},
				map[string]any{"path": ".env.b", "required": false},
			},
			want: []string{".env.a", ".env.b"},
		},
		{
			name: "object with empty path skipped",
			in: []any{
				map[string]any{"path": "", "required": true},
				map[string]any{"path": ".env.a"},
			},
			want: []string{".env.a"},
		},
		{name: "unsupported type", in: 42, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, envFilePaths(tt.in))
		})
	}
}

// TestFileTypeString covers the human-readable label used in the validator's
// error message. The test pins the label for each special-file mode bit so a
// future change to the label set is an explicit decision, not an accident.
func TestFileTypeString(t *testing.T) {
	assert.Equal(t, "named pipe (FIFO)", fileTypeString(os.ModeNamedPipe))
	assert.Equal(t, "socket", fileTypeString(os.ModeSocket))
	assert.Equal(t, "directory", fileTypeString(os.ModeDir))
	assert.Equal(t, "character device", fileTypeString(os.ModeCharDevice|os.ModeDevice))
	assert.Equal(t, "block device", fileTypeString(os.ModeDevice))
	assert.Equal(t, "symlink", fileTypeString(os.ModeSymlink))
}
