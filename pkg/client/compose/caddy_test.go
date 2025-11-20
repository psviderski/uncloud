package compose

import (
	"context"
	"os"
	"testing"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaddyExtension(t *testing.T) {
	tests := []struct {
		name        string
		composeYAML string
		wantConfig  string
		wantErr     string
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
			wantConfig: `example.com {
  reverse_proxy web:80
}`,
		},
		{
			name: "x-caddy as string with extra spaces",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: |+

      example.com {
        reverse_proxy web:80
      }


`,
			wantConfig: `example.com {
  reverse_proxy web:80
}`,
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
			wantConfig: `example.com {
  reverse_proxy web:80
}`,
		},
		{
			name: "x-caddy as object with config field and extra spaces",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy:
      config: |+

        example.com {
          reverse_proxy web:80
        }


`,
			wantConfig: `example.com {
  reverse_proxy web:80
}`,
		},
		{
			name: "x-caddy with path to Caddyfile",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: testdata/Caddyfile
`,
			wantConfig: `test.example.com {
  reverse_proxy test:8000
}`,
		},
		{
			name: "x-caddy with empty object",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: {}
`,
			wantConfig: "",
		},
		{
			name: "x-caddy with empty string",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: ""
`,
			wantConfig: "",
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
			wantErr: "invalid keys: unknown_field",
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
			wantErr: "expected type 'string'",
		},
		{
			name: "x-caddy with ingress x-ports conflict",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: |
      example.com {
        reverse_proxy web:80
      }
    x-ports:
      - example.com:80/http
`,
			wantErr: "ingress ports in 'x-ports' and 'x-caddy' cannot be specified simultaneously",
		},
		{
			name: "x-caddy with host-only x-ports allowed",
			composeYAML: `
services:
  web:
    image: nginx
    x-caddy: |
      example.com {
        reverse_proxy web:80
      }
    x-ports:
      - 8080:80@host
      - 9090:90/tcp@host
`,
			wantConfig: `example.com {
  reverse_proxy web:80
}`,
			// Should not error - host ports are allowed with x-caddy
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get current working directory for relative path resolution to testdata directory.
			wd, err := os.Getwd()
			require.NoError(t, err)

			project, err := LoadProjectFromContent(
				context.Background(),
				tt.composeYAML,
				composecli.WithWorkingDirectory(wd),
			)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
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

			assert.Equal(t, tt.wantConfig, caddy.Config)
		})
	}
}

func TestIsCaddyfilePath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Should be detected as file paths.
		{"relative path with slash", "./Caddyfile", true},
		{"relative path parent", "../Caddyfile", true},
		{"relative path", "relative/path/to/file", true},
		{"absolute path", "/etc/caddy/Caddyfile", true},
		{"just Caddyfile", "Caddyfile", true},
		{"Caddyfile with suffix", "Caddyfile.app", true},
		{"caddyfile lowercase", "caddyfile", true},
		{"with .caddyfile extension", "my.caddyfile", true},
		{"with .Caddyfile extension", "my.Caddyfile", true},
		{"with .caddy extension", "config.caddy", true},
		{"with .conf extension", "caddy.conf", true},
		{"simple filename", "config", true},

		// Should NOT be detected as file paths.
		{"multiline config", "example.com {\n  reverse_proxy :8080\n}", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCaddyfilePath(tt.input)
			assert.Equal(t, tt.want, result, "isCaddyfilePath(%q) should be %v", tt.input, tt.want)
		})
	}
}
