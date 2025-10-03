package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestEnsureHostPaths(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test file to simulate existing file scenario
	testFile := filepath.Join(tempDir, "existing-file.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a test directory to simulate existing directory scenario
	existingDir := filepath.Join(tempDir, "existing-directory")
	err = os.MkdirAll(existingDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	tests := []struct {
		name     string
		volumes  []api.VolumeSpec
		wantDirs []string
		wantErr  bool
	}{
		{
			name: "bind mount with CreateHostPath=true creates directory",
			volumes: []api.VolumeSpec{
				{
					Name: "test-bind",
					Type: api.VolumeTypeBind,
					BindOptions: &api.BindOptions{
						HostPath:       filepath.Join(tempDir, "test-bind-dir"),
						CreateHostPath: true,
					},
				},
			},
			wantDirs: []string{filepath.Join(tempDir, "test-bind-dir")},
			wantErr:  false,
		},
		{
			name: "bind mount with CreateHostPath=false does not create directory",
			volumes: []api.VolumeSpec{
				{
					Name: "test-bind-no-create",
					Type: api.VolumeTypeBind,
					BindOptions: &api.BindOptions{
						HostPath:       filepath.Join(tempDir, "test-no-create-dir"),
						CreateHostPath: false,
					},
				},
			},
			wantDirs: []string{},
			wantErr:  false,
		},
		{
			name: "volume mount is ignored",
			volumes: []api.VolumeSpec{
				{
					Name: "test-volume",
					Type: api.VolumeTypeVolume,
					VolumeOptions: &api.VolumeOptions{
						Name: "test-volume",
					},
				},
			},
			wantDirs: []string{},
			wantErr:  false,
		},
		{
			name: "mixed bind and volume mounts",
			volumes: []api.VolumeSpec{
				{
					Name: "test-bind-create",
					Type: api.VolumeTypeBind,
					BindOptions: &api.BindOptions{
						HostPath:       filepath.Join(tempDir, "mixed-test-create"),
						CreateHostPath: true,
					},
				},
				{
					Name: "test-volume-ignored",
					Type: api.VolumeTypeVolume,
					VolumeOptions: &api.VolumeOptions{
						Name: "test-volume-ignored",
					},
				},
				{
					Name: "test-bind-no-create",
					Type: api.VolumeTypeBind,
					BindOptions: &api.BindOptions{
						HostPath:       filepath.Join(tempDir, "mixed-test-no-create"),
						CreateHostPath: false,
					},
				},
			},
			wantDirs: []string{filepath.Join(tempDir, "mixed-test-create")},
			wantErr:  false,
		},
		{
			name: "nested directory creation",
			volumes: []api.VolumeSpec{
				{
					Name: "nested-bind",
					Type: api.VolumeTypeBind,
					BindOptions: &api.BindOptions{
						HostPath:       filepath.Join(tempDir, "level1", "level2", "level3"),
						CreateHostPath: true,
					},
				},
			},
			wantDirs: []string{filepath.Join(tempDir, "level1", "level2", "level3")},
			wantErr:  false,
		},
		{
			name: "existing directory is not modified",
			volumes: []api.VolumeSpec{
				{
					Name: "existing-dir",
					Type: api.VolumeTypeBind,
					BindOptions: &api.BindOptions{
						HostPath:       existingDir,
						CreateHostPath: true,
					},
				},
			},
			wantDirs: []string{}, // Directory already exists, no action needed
			wantErr:  false,
		},
		{
			name: "existing temp file is not modified",
			volumes: []api.VolumeSpec{
				{
					Name: "temp-existing-file",
					Type: api.VolumeTypeBind,
					BindOptions: &api.BindOptions{
						HostPath:       testFile,
						CreateHostPath: true,
					},
				},
			},
			wantDirs: []string{}, // No directories should be created
			wantErr:  false,
		},
		{
			name: "empty host path is ignored",
			volumes: []api.VolumeSpec{
				{
					Name: "empty-path",
					Type: api.VolumeTypeBind,
					BindOptions: &api.BindOptions{
						HostPath:       "",
						CreateHostPath: true,
					},
				},
			},
			wantDirs: []string{},
			wantErr:  false,
		},
		{
			name: "nil bind options is ignored",
			volumes: []api.VolumeSpec{
				{
					Name:        "nil-options",
					Type:        api.VolumeTypeBind,
					BindOptions: nil,
				},
			},
			wantDirs: []string{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any existing directories from previous tests
			for _, dir := range tt.wantDirs {
				os.RemoveAll(dir)
			}

			err := ensureHostPaths(tt.volumes)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Check that expected directories were created
			for _, dir := range tt.wantDirs {
				stat, err := os.Stat(dir)
				assert.NoError(t, err, "Expected directory %s to exist", dir)
				assert.True(t, stat.IsDir(), "Expected %s to be a directory", dir)

				// Check permissions (0755)
				assert.Equal(t, os.FileMode(0o755), stat.Mode().Perm(), "Expected directory %s to have permissions 0755", dir)
			}

			// Verify that non-CreateHostPath directories were not created
			nonCreatePaths := []string{
				filepath.Join(tempDir, "test-no-create-dir"),
				filepath.Join(tempDir, "mixed-test-no-create"),
			}
			for _, path := range nonCreatePaths {
				_, err := os.Stat(path)
				assert.True(t, os.IsNotExist(err), "Expected directory %s to not exist", path)
			}
		})
	}
}
