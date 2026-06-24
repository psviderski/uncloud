package osinfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOSRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    map[string]string
	}{
		{
			name: "ubuntu",
			content: `PRETTY_NAME="Ubuntu 24.04.4 LTS"
NAME="Ubuntu"
VERSION_ID="24.04"
VERSION="24.04.4 LTS (Noble Numbat)"
VERSION_CODENAME=noble
ID=ubuntu`,
			want: map[string]string{
				"PRETTY_NAME":      "Ubuntu 24.04.4 LTS",
				"NAME":             "Ubuntu",
				"VERSION_ID":       "24.04",
				"VERSION":          "24.04.4 LTS (Noble Numbat)",
				"VERSION_CODENAME": "noble",
				"ID":               "ubuntu",
			},
		},
		{
			name: "debian",
			content: `PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
NAME="Debian GNU/Linux"
VERSION_ID="13"
VERSION="13 (trixie)"
VERSION_CODENAME=trixie
ID=debian`,
			want: map[string]string{
				"PRETTY_NAME":      "Debian GNU/Linux 13 (trixie)",
				"NAME":             "Debian GNU/Linux",
				"VERSION_ID":       "13",
				"VERSION":          "13 (trixie)",
				"VERSION_CODENAME": "trixie",
				"ID":               "debian",
			},
		},
		{
			name: "alpine",
			content: `NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.20.0
PRETTY_NAME="Alpine Linux v3.20"`,
			want: map[string]string{
				"NAME":        "Alpine Linux",
				"ID":          "alpine",
				"VERSION_ID":  "3.20.0",
				"PRETTY_NAME": "Alpine Linux v3.20",
			},
		},
		{
			name: "comments and blank lines are skipped",
			content: `# This is a comment

ID=ubuntu

# Another comment
PRETTY_NAME='Ubuntu 24.04.4 LTS'`,
			want: map[string]string{
				"ID":          "ubuntu",
				"PRETTY_NAME": "Ubuntu 24.04.4 LTS",
			},
		},
		{
			name:    "empty",
			content: "",
			want:    map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseOSRelease(strings.NewReader(tt.content)))
		})
	}
}

func TestBuildPrettyName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rel           map[string]string
		debianVersion string
		want          string
	}{
		{
			name: "ubuntu uses pretty name with point release",
			rel:  map[string]string{"ID": "ubuntu", "PRETTY_NAME": "Ubuntu 24.04.4 LTS"},
			want: "Ubuntu 24.04.4 LTS",
		},
		{
			name:          "debian uses debian_version for the point release",
			rel:           map[string]string{"ID": "debian", "PRETTY_NAME": "Debian GNU/Linux 13 (trixie)"},
			debianVersion: "13.5",
			want:          "Debian 13.5",
		},
		{
			name:          "debian unstable uses the codename from debian_version",
			rel:           map[string]string{"ID": "debian", "PRETTY_NAME": "Debian GNU/Linux 13 (trixie)"},
			debianVersion: "trixie/sid",
			want:          "Debian trixie/sid",
		},
		{
			name: "alpine uses pretty name",
			rel:  map[string]string{"ID": "alpine", "PRETTY_NAME": "Alpine Linux v3.20"},
			want: "Alpine Linux v3.20",
		},
		{
			name: "fallback composes name, version and codename without pretty name",
			rel:  map[string]string{"NAME": "Foo Linux", "VERSION_ID": "1.2", "VERSION_CODENAME": "bar"},
			want: "Foo Linux 1.2 (bar)",
		},
		{
			name: "empty map",
			rel:  map[string]string{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildPrettyName(tt.rel, tt.debianVersion))
		})
	}
}

func TestPrettyName_MissingFile(t *testing.T) {
	// Not parallel: it mutates the package-level path variables.
	original := osReleasePaths
	defer func() { osReleasePaths = original }()

	osReleasePaths = []string{filepath.Join(t.TempDir(), "nonexistent-os-release")}
	assert.Equal(t, "", PrettyName())
}

func TestPrettyName_DebianPointRelease(t *testing.T) {
	// Not parallel: it mutates the package-level path variables.
	originalRelease, originalDebian := osReleasePaths, debianVersionPath
	defer func() {
		osReleasePaths = originalRelease
		debianVersionPath = originalDebian
	}()

	dir := t.TempDir()
	releasePath := filepath.Join(dir, "os-release")
	require.NoError(t, os.WriteFile(releasePath, []byte(`ID=debian
PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
VERSION_ID="13"`), 0o644))
	debianPath := filepath.Join(dir, "debian_version")
	require.NoError(t, os.WriteFile(debianPath, []byte("13.5\n"), 0o644))

	osReleasePaths = []string{releasePath}
	debianVersionPath = debianPath

	assert.Equal(t, "Debian 13.5", PrettyName())
}
