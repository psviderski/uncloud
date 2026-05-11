package compose

import (
	"context"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreDeployHookExtension(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    PreDeployHook
		wantErr string
	}{
		{
			name: "command only",
			yaml: `
services:
  web:
    image: nginx
    x-pre_deploy:
      command: ["echo", "hello"]
`,
			want: PreDeployHook{
				Command: types.ShellCommand{"echo", "hello"},
			},
		},
		{
			name: "command as string",
			yaml: `
services:
  web:
    image: nginx
    x-pre_deploy:
      command: echo hello
`,
			want: PreDeployHook{
				Command: types.ShellCommand{"echo", "hello"},
			},
		},
		// TODO: explore ways to error on unknown attributes instead of ignoring them.
		{
			name: "all attributes",
			yaml: `
services:
  web:
    image: nginx
    x-pre_deploy:
      command: ["sh", "-c", "migrate up"]
      environment:
        DB_HOST: localhost
        DB_PORT: "5432"
      privileged: true
      timeout: 2m30s
      user: root
      unknown_attribute: should be ignored
`,
			want: PreDeployHook{
				Command: types.ShellCommand{"sh", "-c", "migrate up"},
				Environment: types.MappingWithEquals{
					"DB_HOST": new("localhost"),
					"DB_PORT": new("5432"),
				},
				Privileged: new(true),
				Timeout:    new(types.Duration(2*time.Minute + 30*time.Second)),
				User:       "root",
			},
		},
		{
			name: "timeout as seconds",
			yaml: `
services:
  web:
    image: nginx
    x-pre_deploy:
      command: ["true"]
      timeout: 30s
`,
			want: PreDeployHook{
				Command: types.ShellCommand{"true"},
				Timeout: new(types.Duration(30 * time.Second)),
			},
		},
		{
			name: "privileged false",
			yaml: `
services:
  web:
    image: nginx
    x-pre_deploy:
      command: ["true"]
      privileged: false
`,
			want: PreDeployHook{
				Command:    types.ShellCommand{"true"},
				Privileged: new(false),
			},
		},
		{
			name: "missing command should fail",
			yaml: `
services:
  web:
    image: nginx
    x-pre_deploy:
      user: root
`,
			wantErr: "missing required attribute 'command'",
		},
		{
			name: "empty command should fail",
			yaml: `
services:
  web:
    image: nginx
    x-pre_deploy:
      command: []
`,
			wantErr: "missing required attribute 'command'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := LoadProjectFromContent(context.Background(), tt.yaml)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			service, err := project.GetService("web")
			require.NoError(t, err)

			ext, ok := service.Extensions[PreDeployHookExtensionKey]
			require.True(t, ok, "x-pre_deploy extension not found")

			hook, ok := ext.(PreDeployHook)
			require.True(t, ok, "x-pre_deploy extension is not PreDeployHook type")
			assert.Equal(t, tt.want, hook)
		})
	}
}
