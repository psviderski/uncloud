package gitutil

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GitState contains information about the Git repository state.
type GitState struct {
	// IsDirty indicates whether there are uncommitted changes.
	IsDirty bool
	// IsRepo indicates whether the directory is a Git repository.
	IsRepo bool
	// SHA is the full SHA-1 (40 characters) of the current commit.
	SHA string
	// Time is the current commit datetime.
	Time time.Time
}

// InspectGitState inspects the Git repo state from the specified directory.
// If the directory is not a Git repository, or if git utility is not available,
// returns GitState with IsRepo=false and no error.
func InspectGitState(dir string) (GitState, error) {
	// Check if git command is available.
	if _, err := exec.LookPath("git"); err != nil {
		return GitState{}, nil
	}

	state := GitState{
		IsRepo: isGitRepo(dir),
	}
	if !state.IsRepo {
		return state, nil
	}

	// Get the current commit SHA.
	sha, err := gitCommand(dir, "rev-parse", "HEAD")
	if err != nil {
		return state, fmt.Errorf("get current commit SHA: %w", err)
	}
	state.SHA = strings.TrimSpace(sha)

	// Get the current commit timestamp.
	timestamp, err := gitCommand(dir, "log", "-1", "--format=%ct")
	if err != nil {
		return state, fmt.Errorf("get current commit timestamp: %w", err)
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return state, fmt.Errorf("parse current commit timestamp: %w", err)
	}
	state.Time = time.Unix(seconds, 0)

	// Check for uncommitted changes.
	status, err := gitCommand(dir, "status", "--porcelain")
	if err != nil {
		return state, fmt.Errorf("check git status: %w", err)
	}
	state.IsDirty = strings.TrimSpace(status) != ""

	return state, nil
}

// isGitRepo checks if the directory is a Git repository.
func isGitRepo(dir string) bool {
	_, err := gitCommand(dir, "rev-parse", "--git-dir")
	return err == nil
}

// gitCommand runs a git command in the specified directory.
func gitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git command failed: %s", exitErr.Stderr)
		}
		return "", err
	}

	return string(output), nil
}

// ShortSHA returns a truncated SHA with the specified length.
// If length is negative, zero, or greater than the SHA length, it returns the full SHA.
// Returns empty string if SHA is empty.
func (s *GitState) ShortSHA(length int) string {
	if length <= 0 || length > len(s.SHA) {
		length = len(s.SHA)
	}
	return s.SHA[:length]
}
