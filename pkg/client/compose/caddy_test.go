package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaddyExtension(t *testing.T) {
	tests := []struct {
		name           string
		composeYAML    string
		expectedConfig string
		wantErr        bool
	}{
		{
			name: "x-caddy as string",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: |
      example.com {
        reverse_proxy web:80
      }
`,
			expectedConfig: `example.com {
  reverse_proxy web:80
}
`,
		},
		{
			name: "x-caddy as object with config field",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy:
      config: |
        example.com {
          reverse_proxy web:80
        }
`,
			expectedConfig: `example.com {
  reverse_proxy web:80
}
`,
		},

		{
			name: "x-caddy with empty object",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: {}
`,
			expectedConfig: "",
		},
		{
			name: "x-caddy with empty string",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: ""
`,
			expectedConfig: "",
		},
		{
			name: "x-caddy with extra unknown field should fail",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy:
      config: |
        example.com {
          reverse_proxy web:80
        }
      unknown_field: "should cause error"
`,
			wantErr: true,
		},
		{
			name: "x-caddy with non-string config field should fail",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy:
      config: 123
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := loadProjectFromContent(t, tt.composeYAML)

			if tt.wantErr {
				require.Error(t, err, "expected error for test case with invalid extension")
				return
			}

			require.NoError(t, err)

			service, err := project.GetService("web")
			require.NoError(t, err)

			// Verify the x-caddy extension was parsed correctly.
			caddyExt, ok := service.Extensions[CaddyExtensionKey]
			require.True(t, ok, "x-caddy extension not found")

			caddy, ok := caddyExt.(Caddy)
			require.True(t, ok, "x-caddy extension is not Caddy type")

			assert.Equal(t, tt.expectedConfig, caddy.Config)
		})
	}
}
