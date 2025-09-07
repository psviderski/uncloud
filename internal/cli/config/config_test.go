package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_Save(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for the test
	tmpDir := t.TempDir()

	// Change to temp directory so relative paths resolve correctly
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Logf("Failed to restore original directory: %v", err)
		}
	}()

	tests := []struct {
		name            string
		configPath      string
		contextName     string
		expectFileAt    string // Expected file location for verification
		useAbsolutePath bool   // Whether to use absolute path for expectFileAt
	}{
		{
			name:        "relative path without prefix",
			configPath:  "test-config.yaml",
			contextName: "test",
		},
		{
			name:        "relative path with prefix",
			configPath:  "./test-config-2.yaml",
			contextName: "test2",
		},
		{
			name:        "absolute path",
			configPath:  filepath.Join(tmpDir, "absolute-config.yaml"),
			contextName: "test3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := os.Chdir(tmpDir); err != nil {
				t.Fatalf("Failed to change to temp directory: %v", err)
			}

			cfg := &Config{
				CurrentContext: tt.contextName,
				Contexts: map[string]*Context{
					tt.contextName: {
						Name: tt.contextName,
					},
				},
				path: tt.configPath,
			}

			// This should not fail when saving the config
			err := cfg.Save()
			if err != nil {
				t.Errorf("Expected no error when saving config, got: %v", err)
			}

			// Verify the file was created
			if _, err := os.Stat(tt.configPath); os.IsNotExist(err) {
				t.Errorf("Config file was not created at expected path: %s", tt.configPath)
			}
		})
	}
}
