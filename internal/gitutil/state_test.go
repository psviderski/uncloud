package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInspectGitState_GitNotAvailable(t *testing.T) {
	// Reset the PATH env var to simulate git not being available.
	t.Setenv("PATH", "")

	tmpDir := t.TempDir()

	state, err := InspectGitState(tmpDir)
	require.NoError(t, err)

	assert.False(t, state.IsRepo)
	assert.Empty(t, state.SHA)
	assert.Empty(t, state.ShortSHA(7))
	assert.True(t, state.Time.IsZero())
	assert.False(t, state.IsDirty)
}

func TestInspectGitState_NotARepo(t *testing.T) {
	// Create a temporary directory that's not a git repo.
	tmpDir := t.TempDir()

	state, err := InspectGitState(tmpDir)
	require.NoError(t, err)

	// If tmpDir happens to be in a git repo (e.g., parent directory), skip this test.
	// This is common when running tests from within the project itself.
	if state.IsRepo {
		t.Skip("tmpDir is within a git repository, skipping test")
	}

	assert.False(t, state.IsRepo)
	assert.Empty(t, state.SHA)
	assert.Empty(t, state.ShortSHA(7))
	assert.True(t, state.Time.IsZero()) // Should have zero time if not a repo.
	assert.False(t, state.IsDirty)
}

func TestInspectGitState_CleanRepo(t *testing.T) {
	// Create a temporary git repo.
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	createAndCommitFile(t, tmpDir, "test.txt", "test content")

	state, err := InspectGitState(tmpDir)
	require.NoError(t, err)

	assert.True(t, state.IsRepo)
	assert.Len(t, state.SHA, 40)
	assert.Equal(t, state.ShortSHA(7), state.SHA[:7])
	assert.False(t, state.Time.IsZero())
	assert.False(t, state.IsDirty)
}

func TestInspectGitState_DirtyRepo(t *testing.T) {
	// Create a temporary git repo.
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	createAndCommitFile(t, tmpDir, "test.txt", "test content")

	// Modify the file to make the repo dirty.
	err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("modified content"), 0o644)
	require.NoError(t, err)

	state, err := InspectGitState(tmpDir)
	require.NoError(t, err)

	assert.True(t, state.IsRepo)
	assert.True(t, state.IsDirty)
}

func TestInspectGitState_UntrackedFiles(t *testing.T) {
	// Create a temporary git repo.
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	createAndCommitFile(t, tmpDir, "test.txt", "test content")

	// Add an untracked file.
	err := os.WriteFile(filepath.Join(tmpDir, "untracked.txt"), []byte("untracked"), 0o644)
	require.NoError(t, err)

	state, err := InspectGitState(tmpDir)
	require.NoError(t, err)

	assert.True(t, state.IsRepo)
	assert.True(t, state.IsDirty) // Untracked files make the repo dirty.
}

func TestGitState_ShortSHA(t *testing.T) {
	state := &GitState{
		SHA: "1234567890abcdef1234567890abcdef12345678",
	}

	tests := []struct {
		name     string
		length   int
		expected string
	}{
		{"short 7", 7, "1234567"},
		{"short 10", 10, "1234567890"},
		{"full SHA", 40, "1234567890abcdef1234567890abcdef12345678"},
		{"longer than SHA", 50, "1234567890abcdef1234567890abcdef12345678"},
		{"negative", -42, "1234567890abcdef1234567890abcdef12345678"},
		{"zero", 0, "1234567890abcdef1234567890abcdef12345678"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := state.ShortSHA(tt.length)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitState_ShortSHA_Empty(t *testing.T) {
	state := &GitState{
		SHA: "",
	}

	result := state.ShortSHA(7)
	assert.Empty(t, result)
}

// Helper functions.

func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	// Initialize git repo.
	runGitCommand(t, dir, "init")
	runGitCommand(t, dir, "config", "user.email", "test@example.com")
	runGitCommand(t, dir, "config", "user.name", "Test User")
}

func createAndCommitFile(t *testing.T, dir, filename, content string) {
	t.Helper()

	// Create file.
	path := filepath.Join(dir, filename)
	err := os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err)

	// Commit file.
	runGitCommand(t, dir, "add", filename)
	runGitCommand(t, dir, "commit", "-m", "Add "+filename)
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git command failed: %s", output)
}
