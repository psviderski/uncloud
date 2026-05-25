package context

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestCommandArgsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		newCmd  func() *cobra.Command
		args    []string
		wantErr bool
	}{
		{
			name:    "root accepts no args",
			newCmd:  NewRootCommand,
			args:    nil,
			wantErr: false,
		},
		{
			name:    "root rejects extra args",
			newCmd:  NewRootCommand,
			args:    []string{"extra"},
			wantErr: true,
		},
		{
			name:    "list accepts no args",
			newCmd:  NewListCommand,
			args:    nil,
			wantErr: false,
		},
		{
			name:    "list rejects extra args",
			newCmd:  NewListCommand,
			args:    []string{"extra"},
			wantErr: true,
		},
		{
			name:    "show accepts no args",
			newCmd:  NewShowCommand,
			args:    nil,
			wantErr: false,
		},
		{
			name:    "show rejects extra args",
			newCmd:  NewShowCommand,
			args:    []string{"extra"},
			wantErr: true,
		},
		{
			name:    "connection accepts no args",
			newCmd:  NewConnectionCommand,
			args:    nil,
			wantErr: false,
		},
		{
			name:    "connection rejects extra args",
			newCmd:  NewConnectionCommand,
			args:    []string{"extra"},
			wantErr: true,
		},
		{
			name:    "use accepts no args",
			newCmd:  NewUseCommand,
			args:    nil,
			wantErr: false,
		},
		{
			name:    "use accepts one arg",
			newCmd:  NewUseCommand,
			args:    []string{"prod"},
			wantErr: false,
		},
		{
			name:    "use rejects extra args",
			newCmd:  NewUseCommand,
			args:    []string{"prod", "extra"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := tt.newCmd()
			require.NotNil(t, cmd.Args)

			err := cmd.Args(cmd, tt.args)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}
