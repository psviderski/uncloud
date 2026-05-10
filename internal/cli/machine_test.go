package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstallCmd(t *testing.T) {
	const scriptB64 = "SCRIPT_BASE64_PLACEHOLDER"

	tests := []struct {
		name    string
		user    string
		version string
		want    string
	}{
		{
			name: "root",
			user: "root",
			want: "printf '%s' SCRIPT_BASE64_PLACEHOLDER | base64 -d | bash",
		},
		{
			name:    "root with version",
			user:    "root",
			version: "v1.2.3",
			want:    "printf '%s' SCRIPT_BASE64_PLACEHOLDER | base64 -d | UNCLOUD_VERSION=v1.2.3 bash",
		},
		{
			name: "nonroot",
			user: "nonroot",
			want: "printf '%s' SCRIPT_BASE64_PLACEHOLDER | base64 -d | sudo UNCLOUD_GROUP_ADD_USER=nonroot bash",
		},
		{
			name:    "nonroot with version",
			user:    "nonroot",
			version: "v1.2.3",
			want: "printf '%s' SCRIPT_BASE64_PLACEHOLDER | base64 -d | " +
				"sudo UNCLOUD_GROUP_ADD_USER=nonroot UNCLOUD_VERSION=v1.2.3 bash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, installCmd(scriptB64, tt.user, tt.version))
		})
	}
}
