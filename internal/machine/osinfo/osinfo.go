// Package osinfo collects the host operating system information.
package osinfo

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// Paths to the files describing the OS release. They are package variables so tests can override them.
var (
	osReleasePaths    = []string{"/etc/os-release", "/usr/lib/os-release"}
	debianVersionPath = "/etc/debian_version"
)

// PrettyName returns a human-readable OS name and version derived from os-release, or an empty string
// if it cannot be determined. For example, "Ubuntu 24.04.4 LTS" or "Debian 13.5".
func PrettyName() string {
	rel := readOSRelease()
	if len(rel) == 0 {
		return ""
	}

	// Debian's os-release only carries the major version (e.g. "13"), while /etc/debian_version
	// holds the point release (e.g. "13.5"). Read it to report the precise version.
	var debianVersion string
	if rel["ID"] == "debian" {
		if data, err := os.ReadFile(debianVersionPath); err == nil {
			debianVersion = strings.TrimSpace(string(data))
		}
	}

	return buildPrettyName(rel, debianVersion)
}

// readOSRelease reads the first available os-release file and parses it into a key-value map. It
// returns an empty map if no file is found.
func readOSRelease() map[string]string {
	for _, path := range osReleasePaths {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		rel := parseOSRelease(f)
		f.Close()
		return rel
	}
	return nil
}

// parseOSRelease parses an os-release file into a key-value map. Blank lines and comments are
// skipped, and surrounding quotes are stripped from values.
func parseOSRelease(r io.Reader) map[string]string {
	rel := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		rel[key] = value
	}
	return rel
}

// buildPrettyName composes a human-readable OS name and version from the parsed os-release map and the
// contents of /etc/debian_version (empty for non-Debian systems).
func buildPrettyName(rel map[string]string, debianVersion string) string {
	// Debian's PRETTY_NAME is verbose ("Debian GNU/Linux 13 (trixie)") and lacks the point release,
	// so prefer the precise version or codename (unstable releases) from /etc/debian_version.
	if rel["ID"] == "debian" {
		return strings.Join([]string{"Debian", debianVersion}, " ")
	}

	// PRETTY_NAME is the vendor's human-readable string and already includes the point release on
	// most distros (e.g. "Ubuntu 24.04.4 LTS").
	if pretty := rel["PRETTY_NAME"]; pretty != "" {
		return pretty
	}

	// Fall back to composing the name from the individual fields.
	name := rel["NAME"]
	if name == "" {
		name = rel["ID"]
	}
	parts := make([]string, 0, 3)
	if name != "" {
		parts = append(parts, name)
	}
	if version := rel["VERSION_ID"]; version != "" {
		parts = append(parts, version)
	}
	if codename := rel["VERSION_CODENAME"]; codename != "" {
		parts = append(parts, "("+codename+")")
	}
	return strings.Join(parts, " ")
}
