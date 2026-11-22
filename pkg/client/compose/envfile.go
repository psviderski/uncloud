package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// composeEnvFileDoc is the subset of a Compose file needed to check env_file targets before the
// loader opens them. Opening a named pipe blocks until the other end is opened, so a FIFO left
// behind by an interrupted command would hang the CLI with no output.
type composeEnvFileDoc struct {
	Services map[string]struct {
		EnvFile any `yaml:"env_file"`
	} `yaml:"services"`
}

// checkEnvFileTypes returns an error if any env_file referenced by the Compose files is not a
// regular file. Missing files are left to the loader, which reports them (or ignores them when
// env_file is marked as not required).
func checkEnvFileTypes(projectDir string, configPaths []string) error {
	if projectDir == "" && len(configPaths) > 0 {
		// Match the loader, which infers the project directory from the first Compose file.
		projectDir = filepath.Dir(configPaths[0])
	}

	for _, configPath := range configPaths {
		if err := checkEnvFileTypesInFile(projectDir, configPath); err != nil {
			return err
		}
	}
	return nil
}

func checkEnvFileTypesInFile(projectDir, configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		// Let the loader report a missing or unreadable Compose file.
		return nil
	}

	var doc composeEnvFileDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		// Let the loader report invalid YAML with its own error.
		return nil
	}

	for name, service := range doc.Services {
		for _, envFile := range envFilePaths(service.EnvFile) {
			path := envFile
			if !filepath.IsAbs(path) {
				path = filepath.Join(projectDir, path)
			}

			info, err := os.Stat(path)
			if err != nil || info.Mode().IsRegular() {
				continue
			}

			return fmt.Errorf("service '%s': env_file '%s' is not a regular file (got: %s)",
				name, envFile, fileType(info.Mode()))
		}
	}

	return nil
}

// envFilePaths extracts the paths from an env_file attribute, which the Compose spec allows as a
// single string, a list of strings, or a list of objects with a path.
func envFilePaths(value any) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []any:
		paths := make([]string, 0, len(v))
		for _, item := range v {
			switch entry := item.(type) {
			case string:
				paths = append(paths, entry)
			case map[string]any:
				if path, ok := entry["path"].(string); ok {
					paths = append(paths, path)
				}
			}
		}
		return paths
	case map[string]any:
		if path, ok := v["path"].(string); ok {
			return []string{path}
		}
	}
	return nil
}

func fileType(mode os.FileMode) string {
	switch {
	case mode.IsDir():
		return "directory"
	case mode&os.ModeNamedPipe != 0:
		return "fifo"
	case mode&os.ModeSocket != 0:
		return "socket"
	case mode&os.ModeDevice != 0:
		return "device"
	default:
		return "irregular file"
	}
}
