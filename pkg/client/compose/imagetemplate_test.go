package compose

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/internal/gitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessImageTemplates_Integration(t *testing.T) {
	// Create a temporary directory for the test git repository.
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create an initial commit.
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("initial content"), 0644)
	require.NoError(t, err)
	runGitCommand(t, tmpDir, "add", "test.txt")
	runGitCommand(t, tmpDir, "commit", "-m", "Initial commit")

	gitState, err := gitutil.InspectGitState(tmpDir)
	require.NoError(t, err)
	require.True(t, gitState.IsRepo)
	require.False(t, gitState.IsDirty)
	require.NotEmpty(t, gitState.SHA)

	expectedTag := gitState.Date.UTC().Format("2006-01-02-150405") + "." + gitState.SHA[:7]
	expectedDate := gitState.Date.UTC().Format("2006-01-02")

	t.Run("clean repository", func(t *testing.T) {
		project := &types.Project{
			Name:       "myproject",
			WorkingDir: tmpDir,
			Services: types.Services{
				"service1": {
					Name:  "service1",
					Image: "myapp:{{.Tag}}",
					Build: &types.BuildConfig{Context: "."},
				},
				"service2": {
					Name:  "service2",
					Image: `myapp:v1.0-{{gitsha 7}}`,
					Build: &types.BuildConfig{Context: "."},
				},
				"service3": {
					Name:  "service3",
					Image: "myapp", // No tag, has build, should get default tag template appended.
					Build: &types.BuildConfig{Context: "."},
				},
				"service4": {
					Name:  "service4",
					Image: "", // Empty image, has build, should use default template.
					Build: &types.BuildConfig{Context: "."},
				},
				"service5": {
					Name:  "service5",
					Image: "{{.Project}}/{{.Service}}:custom", // Custom template.
					Build: &types.BuildConfig{Context: "."},
				},
				"service6": {
					Name:  "service6",
					Image: `registry.example.com/{{.Project}}/{{.Service}}:{{gitdate "2006-01-02"}}-{{gitsha 7}}`,
					Build: &types.BuildConfig{Context: "."},
				},
				"service7": {
					Name:  "service7",
					Image: `myapp:{{if .Git.IsRepo}}git-{{.Git.SHA}}{{else}}no-git{{end}}`,
					Build: &types.BuildConfig{Context: "."},
				},
				"service8": {
					Name:  "service8",
					Image: `myapp:{{gitdate "20060102"}}.{{gitsha 10}}{{if .Git.IsDirty}}.dirty{{end}}`,
					Build: &types.BuildConfig{Context: "."},
				},
				"postgres": {
					Name:  "postgres",
					Image: "postgres", // No build section, should NOT get tag appended.
				},
				"redis": {
					Name:  "redis",
					Image: "redis:7.2", // Third-party with tag, should be unchanged.
				},
			},
		}

		p, err := ProcessImageTemplates(project)
		require.NoError(t, err)

		assert.Equal(t, "myapp:"+expectedTag, p.Services["service1"].Image)
		assert.Equal(t, "myapp:v1.0-"+gitState.SHA[:7], p.Services["service2"].Image)
		assert.Equal(t, "myapp:"+expectedTag, p.Services["service3"].Image)
		assert.Equal(t, "myproject/service4:"+expectedTag, p.Services["service4"].Image)
		assert.Equal(t, "myproject/service5:custom", p.Services["service5"].Image)
		expectedService6 := "registry.example.com/myproject/service6:" + expectedDate + "-" + gitState.SHA[:7]
		assert.Equal(t, expectedService6, p.Services["service6"].Image)
		assert.Equal(t, "myapp:git-"+gitState.SHA, p.Services["service7"].Image)
		expectedService8 := "myapp:" + gitState.Date.UTC().Format("20060102") + "." + gitState.SHA[:10]
		assert.Equal(t, expectedService8, p.Services["service8"].Image)

		// Images without build section should be unchanged.
		assert.Equal(t, "postgres", p.Services["postgres"].Image, "postgres image should not have tag appended")
		assert.Equal(t, "redis:7.2", p.Services["redis"].Image, "redis image should be unchanged")
	})

	t.Run("dirty repository", func(t *testing.T) {
		// Make the repository dirty by creating an uncommitted file.
		dirtyFile := filepath.Join(tmpDir, "uncommitted.txt")
		err := os.WriteFile(dirtyFile, []byte("uncommitted changes"), 0644)
		require.NoError(t, err)

		dirtyGitState, err := gitutil.InspectGitState(tmpDir)
		require.NoError(t, err)
		require.True(t, dirtyGitState.IsDirty)
		require.Equal(t, gitState.SHA, dirtyGitState.SHA)

		project := &types.Project{
			Name:       "myproject",
			WorkingDir: tmpDir,
			Services: types.Services{
				"default": {
					Name:  "default",
					Build: &types.BuildConfig{Context: "."},
				},
				"app": {
					Name:  "app",
					Image: `myapp:{{gitdate "2006-01-02"}}-{{gitsha 7}}{{if .Git.IsDirty}}.dirty{{end}}`,
					Build: &types.BuildConfig{Context: "."},
				},
			},
		}

		p, err := ProcessImageTemplates(project)
		require.NoError(t, err)

		expectedDirtyDate := dirtyGitState.Date.UTC().Format("2006-01-02")

		assert.Equal(t, "myproject/default:"+expectedTag+".dirty", p.Services["default"].Image)
		expectedApp := "myapp:" + expectedDirtyDate + "-" + dirtyGitState.SHA[:7] + ".dirty"
		assert.Equal(t, expectedApp, p.Services["app"].Image)
	})

	t.Run("non-git directory", func(t *testing.T) {
		nonGitDir := t.TempDir()

		project := &types.Project{
			Name:       "myproject",
			WorkingDir: nonGitDir,
			Services: types.Services{
				"default": {
					Name:  "default",
					Build: &types.BuildConfig{Context: "."},
				},
				"app": {
					Name:  "app",
					Image: `myapp:{{if .Git.IsRepo}}git{{else}}{{date "2006-01-02"}}{{end}}`,
					Build: &types.BuildConfig{Context: "."},
				},
			},
		}

		p, err := ProcessImageTemplates(project)
		require.NoError(t, err)

		assert.Regexp(t, regexp.MustCompile("^myproject/default:"+expectedDate+`-\d{6}$`), p.Services["default"].Image)
		assert.Equal(t, "myapp:"+expectedDate, p.Services["app"].Image)
	})
}

func TestProcessImageTemplate(t *testing.T) {
	// Fixed time for testing: 2025-08-16 13:07:34 UTC.
	fixedTime := time.Date(2025, 8, 16, 13, 7, 34, 0, time.UTC)

	gitState := gitutil.GitState{
		Date:    fixedTime,
		IsDirty: false,
		IsRepo:  true,
		SHA:     "84d33bb1234567890abcdef1234567890abcdef",
	}
	ctx, err := newImageTemplateContext("myproject", "myservice", gitState)
	require.NoError(t, err)

	tests := []struct {
		name     string
		image    string
		expected string
		wantErr  bool
	}{
		{
			name:     "no template markers",
			image:    "myapp:v1.0",
			expected: "myapp:v1.0",
		},
		{
			name:     "git SHA function with length",
			image:    "myapp:{{gitsha 10}}",
			expected: "myapp:84d33bb123",
		},
		{
			name:     "git SHA function full length",
			image:    "myapp:{{gitsha}}",
			expected: "myapp:84d33bb1234567890abcdef1234567890abcdef",
		},
		{
			name:     "git SHA function negative length",
			image:    "myapp:{{gitsha -1}}",
			expected: "myapp:84d33bb1234567890abcdef1234567890abcdef",
		},
		{
			name:     "git date",
			image:    `myapp:{{gitdate "2006-01-02"}}`,
			expected: "myapp:2025-08-16",
		},
		{
			name:     "git date with time",
			image:    `myapp:{{gitdate "2006-01-02-150405"}}`,
			expected: "myapp:2025-08-16-130734",
		},
		{
			name:     "git date with timezone",
			image:    `myapp:{{gitdate "2006-01-02-150405" "Australia/Brisbane"}}`,
			expected: "myapp:2025-08-16-230734",
		},
		{
			name:     "current date",
			image:    `myapp:{{date "2006-01-02"}}`,
			expected: `^myapp:\d{4}-\d{2}-\d{2}$`,
		},
		{
			name:     "current date with time",
			image:    `myapp:{{date "20060102-150405"}}`,
			expected: `^myapp:\d{8}-\d{6}$`,
		},
		{
			name:     "current date with timezone",
			image:    `myapp:{{date "20060102-150405" "Local"}}`,
			expected: `^myapp:\d{8}-\d{6}$`,
		},
		{
			name:     "multiple template functions",
			image:    `myapp:v1.0-{{gitdate "2006-01-02"}}-{{gitsha 7}}`,
			expected: "myapp:v1.0-2025-08-16-84d33bb",
		},
		{
			name:     "project and service variables",
			image:    "{{.Project}}/{{.Service}}:{{gitsha 7}}",
			expected: "myproject/myservice:84d33bb",
		},
		{
			name:     "git SHA variable",
			image:    "myapp:{{.Git.SHA}}",
			expected: "myapp:84d33bb1234567890abcdef1234567890abcdef",
		},
		{
			name:     "tag variable",
			image:    "myapp:{{.Tag}}",
			expected: "myapp:2025-08-16-130734.84d33bb",
		},
		{
			name:     "default image template short",
			image:    "{{.Project}}/{{.Service}}:{{.Tag}}",
			expected: "myproject/myservice:2025-08-16-130734.84d33bb",
		},
		{
			name:     "default image template long",
			image:    `{{.Project}}/{{.Service}}:{{if .Git.IsRepo}}{{gitdate "2006-01-02-150405"}}.{{gitsha 7}}{{if .Git.IsDirty}}.dirty{{end}}{{else}}{{date "2006-01-02-150405"}}{{end}}`,
			expected: "myproject/myservice:2025-08-16-130734.84d33bb",
		},
		{
			name:     "whitespace only",
			image:    "   ",
			expected: "   ",
		},
		{
			name:    "invalid template syntax",
			image:   "myapp:{{.Git.NonExistent.Field}}",
			wantErr: true,
		},
		{
			name:    "unclosed template",
			image:   "myapp:{{.Tag",
			wantErr: true,
		},
		{
			name:    "invalid gitsha argument - not a number",
			image:   `myapp:{{gitsha "abc"}}`,
			wantErr: true,
		},
		{
			name:    "invalid date format - missing quotes",
			image:   "myapp:{{date 2006-01-02}}",
			wantErr: true,
		},
		{
			name:    "division by zero in template",
			image:   "myapp:{{divide 1 0}}",
			wantErr: true,
		},
		{
			name:    "undefined function",
			image:   "myapp:{{undefined_func}}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processImageTemplate(tt.image, ctx)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if strings.HasPrefix(tt.expected, "^") {
					assert.Regexp(t, tt.expected, result)
				} else {
					assert.Equal(t, tt.expected, result)
				}
			}
		})
	}
}

func TestHasTag(t *testing.T) {
	tests := []struct {
		name    string
		image   string
		want    bool
		wantErr bool
	}{
		{
			name:  "image with tag",
			image: "myapp:v1.0",
			want:  true,
		},
		{
			name:  "image without tag",
			image: "myapp",
			want:  false,
		},
		{
			name:  "registry with port and tag",
			image: "localhost:5000/myapp:v1.0",
			want:  true,
		},
		{
			name:  "registry with port, no tag",
			image: "localhost:5000/myapp",
			want:  false,
		},
		{
			name:  "registry path with tag",
			image: "registry.example.com/org/myapp:v1.0",
			want:  true,
		},
		{
			name:  "registry path without tag",
			image: "registry.example.com/org/myapp",
			want:  false,
		},
		{
			name:  "image with digest",
			image: "myapp@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			want:  true,
		},
		{
			name:  "registry with tag and digest",
			image: "registry.example.com/myapp:v1.0@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			want:  true,
		},
		// Invalid image references.
		{
			name:    "image with empty tag",
			image:   "myapp:",
			want:    false,
			wantErr: true,
		},
		{
			name:    "empty string",
			image:   "",
			want:    false,
			wantErr: true,
		},
		{
			name:    "invalid characters - spaces",
			image:   "my app:v1.0",
			want:    false,
			wantErr: true,
		},
		{
			name:    "invalid characters - uppercase in name",
			image:   "MyApp:v1.0",
			want:    false,
			wantErr: true,
		},
		{
			name:    "invalid digest format",
			image:   "myapp@invalid-digest",
			want:    false,
			wantErr: true,
		},
		{
			name:    "double colon",
			image:   "myapp::tag",
			want:    false,
			wantErr: true,
		},
		{
			name:    "starts with slash",
			image:   "/myapp:v1.0",
			want:    false,
			wantErr: true,
		},
		{
			name:    "ends with slash",
			image:   "myapp/:v1.0",
			want:    false,
			wantErr: true,
		},
		{
			name:    "invalid tag characters",
			image:   "myapp:v1.0@latest",
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := hasTag(tt.image)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

// Helper functions.

func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	// Initialize git repo.
	runGitCommand(t, dir, "init")
	runGitCommand(t, dir, "config", "user.email", "test@example.com")
	runGitCommand(t, dir, "config", "user.name", "Test User")
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git command failed: %s", output)
}
