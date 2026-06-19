package cluster

import (
	"testing"

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
		{"invalid chars replaced", "host_name@1", "host_name-1"},
		{"trim surrounding hyphens", "-_host_-", "_host_"},
		{"whitespace trimmed", "  myhost  ", "myhost"},
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
		// An empty or fully invalid hostname falls back to a random "machine-xxxx" name.
		got, err := DefaultMachineName("***", nil)
		require.NoError(t, err)
		assert.Regexp(t, `^machine-[a-zA-Z0-9]{4}$`, got)
	})

	tests := []struct {
		name     string
		hostname string
		existing []string
		want     string
	}{
		{"from hostname", "web-1.example.com", nil, "web-1"},
		{"dedup against existing", "web", []string{"web"}, "web-1"},
		{"dedup multiple", "web", []string{"web", "web-1"}, "web-2"},
		{"sanitized", "My_Host", nil, "my_host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := DefaultMachineName(tt.hostname, tt.existing)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
