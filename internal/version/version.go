package version

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	goversion "github.com/caarlos0/go-version"
)

func String() string {
	if version == "" {
		return "999.0.0-dev"
	}
	return version
}

// Build-time metadata injected via -ldflags by GoReleaser (see .goreleaser.yaml).
// These are package-level so the same paths can be referenced from any binary's ldflags.
var (
	version string
	commit  string
	dirty   string
	date    string
	builtBy string
)

// Info holds version and build metadata for an Uncloud binary.
type Info struct {
	Version   string
	GitCommit string
	GitState  string
	BuildDate string
	BuiltBy   string
	GoVersion string
	Platform  string
}

// String returns the human-readable representation of Info as a tab-aligned key-value block.
func (i Info) String() string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Version:\t%s\n", i.Version)
	fmt.Fprintf(w, "Git commit:\t%s\n", i.GitCommit)
	fmt.Fprintf(w, "Git state:\t%s\n", i.GitState)
	fmt.Fprintf(w, "Build date:\t%s\n", i.BuildDate)
	fmt.Fprintf(w, "Built by:\t%s\n", i.BuiltBy)
	fmt.Fprintf(w, "Go version:\t%s\n", i.GoVersion)
	fmt.Fprintf(w, "Platform:\t%s\n", i.Platform)
	_ = w.Flush()
	return b.String()
}

// JSONString returns the JSON-encoded representation of Info, indented for human readability.
func (i Info) JSONString() (string, error) {
	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal info: %w", err)
	}
	return string(b), nil
}

// GetInfo returns version and build metadata for the current binary. Values injected at build time via
// -ldflags take precedence over the runtime/debug.BuildInfo fields.
func GetInfo() Info {
	vi := goversion.GetVersionInfo(
		func(i *goversion.Info) {
			i.GitVersion = String()
			if commit != "" {
				i.GitCommit = commit
			}
			// The ldflag value is the raw goreleaser {{.IsGitDirty}} boolean string ("true"/"false").
			switch dirty {
			case "true":
				i.GitTreeState = "dirty"
			case "false":
				i.GitTreeState = "clean"
			}
			if date != "" {
				i.BuildDate = date
			}
			if builtBy != "" {
				i.BuiltBy = builtBy
			}
		},
	)
	return Info{
		Version:   vi.GitVersion,
		GitCommit: vi.GitCommit,
		GitState:  vi.GitTreeState,
		BuildDate: vi.BuildDate,
		BuiltBy:   vi.BuiltBy,
		GoVersion: vi.GoVersion,
		Platform:  vi.Platform,
	}
}
