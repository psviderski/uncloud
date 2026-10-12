package cluster

import (
	"strings"
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMachineNameFromHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hostname string
		want     string
	}{
		{"simple", "web", "web"},
		{"fqdn uses first label", "web-1.example.com", "web-1"},
		{"uppercase lowercased", "Web-Server", "web-server"},
		{"invalid chars replaced", "host_name@1", "host-name-1"},
		{"trim surrounding hyphens", "-_host_-", "host"},
		{"whitespace trimmed", "  myhost  ", "myhost"},
		{"long hostname truncated", strings.Repeat("a", 64), strings.Repeat("a", 63)},
		{"truncation trims trailing hyphen", strings.Repeat("a", 62) + "-b", strings.Repeat("a", 62)},
		{"empty", "", ""},
		{"only invalid chars", "@#", ""},
		{"dot only", ".example.com", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, machineNameFromHostname(tt.hostname))
		})
	}
}

func TestDefaultMachineName(t *testing.T) {
	t.Parallel()

	t.Run("falls back to random", func(t *testing.T) {
		for _, hostname := range []string{"***", "rr", "nearest", strings.Repeat("a", 32)} {
			got, err := DefaultMachineName(hostname, nil)
			require.NoError(t, err)
			assert.Regexp(t, `^machine-[a-z0-9]{4}$`, got)
			require.NoError(t, api.ValidateMachineName(got))
		}
	})

	tests := []struct {
		name     string
		hostname string
		existing []string
		want     string
	}{
		{
			name:     "from hostname",
			hostname: "web-1.example.com",
			want:     "web-1",
		},
		{
			name:     "dedup against existing",
			hostname: "web",
			existing: []string{"web"},
			want:     "web-1",
		},
		{
			name:     "dedup multiple",
			hostname: "web",
			existing: []string{"web", "web-1"},
			want:     "web-2",
		},
		{
			name:     "sanitized",
			hostname: "My_Host",
			want:     "my-host",
		},
		{
			name:     "long hostname",
			hostname: strings.Repeat("a", 64),
			want:     strings.Repeat("a", 63),
		},
		{
			name:     "dedup maximum length",
			hostname: strings.Repeat("a", 63),
			existing: []string{strings.Repeat("a", 63)},
			want:     strings.Repeat("a", 61) + "-1",
		},
		{
			name:     "dedup trims trailing hyphen",
			hostname: strings.Repeat("a", 60) + "-bb",
			existing: []string{strings.Repeat("a", 60) + "-bb"},
			want:     strings.Repeat("a", 60) + "-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := DefaultMachineName(tt.hostname, tt.existing)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			require.NoError(t, api.ValidateMachineName(got))
		})
	}
}
