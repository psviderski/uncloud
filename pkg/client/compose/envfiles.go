package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	composecli "github.com/compose-spec/compose-go/v2/cli"
)

// validateEnvFiles loads the Compose YAML model without resolving env_file
// content, then walks every service's env_file references and ensures each
// path points to a regular file. Non-regular files such as named pipes,
// sockets, devices, or directories are rejected with a clear error before
// the underlying loader's open(2) would block on them. See issue #331.
//
// LoadModel returns the merged YAML tree after extends/transform/interpolation
// but before service environment resolution, so it is safe to call even when
// an env_file path is a FIFO.
func validateEnvFiles(ctx context.Context, options *composecli.ProjectOptions) error {
	model, err := options.LoadModel(ctx)
	if err != nil {
		// Defer parse/extends errors to LoadProject so the user sees the
		// canonical compose-go error message rather than a duplicated one.
		return nil
	}
	workingDir, err := options.GetWorkingDir()
	if err != nil {
		return nil
	}

	services, _ := model["services"].(map[string]any)
	for name, raw := range services {
		svc, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for _, p := range envFilePaths(svc["env_file"]) {
			absPath := p
			if !filepath.IsAbs(absPath) {
				absPath = filepath.Join(workingDir, absPath)
			}
			info, err := os.Stat(absPath)
			if err != nil {
				// Missing or unreadable files are surfaced by compose-go with
				// the right `required: false` semantics. We only catch files
				// that exist but would block on open.
				continue
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf(
					"service %q: env_file %q is not a regular file (got %s)",
					name, p, fileTypeString(info.Mode()),
				)
			}
		}
	}
	return nil
}

// envFilePaths extracts the path strings from the three syntactic forms an
// `env_file:` field can take in a Compose file:
//
//	env_file: .env                                          // string
//	env_file: [.env.a, .env.b]                              // list of strings
//	env_file: [{path: .env, required: false}, ...]          // list of objects
//
// Empty entries are skipped silently so an unset/blank value does not produce
// a stat error on the project working directory.
func envFilePaths(v any) []string {
	switch x := v.(type) {
	case string:
		if x == "" {
			return nil
		}
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			switch y := item.(type) {
			case string:
				if y != "" {
					out = append(out, y)
				}
			case map[string]any:
				if p, ok := y["path"].(string); ok && p != "" {
					out = append(out, p)
				}
			}
		}
		return out
	}
	return nil
}

// fileTypeString returns a short human-readable label for a non-regular file
// mode, used in the error message surfaced by validateEnvFiles.
func fileTypeString(m os.FileMode) string {
	switch {
	case m&os.ModeNamedPipe != 0:
		return "named pipe (FIFO)"
	case m&os.ModeSocket != 0:
		return "socket"
	case m.IsDir():
		return "directory"
	case m&os.ModeCharDevice != 0:
		return "character device"
	case m&os.ModeDevice != 0:
		return "block device"
	case m&os.ModeSymlink != 0:
		return "symlink"
	}
	return fmt.Sprintf("non-regular file (mode %s)", m)
}
