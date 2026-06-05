package machine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeExecArgs(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantMachineName string
		wantCommand     []string
	}{
		{
			name:            "machine only",
			args:            []string{"machine1"},
			wantMachineName: "machine1",
			wantCommand:     []string{},
		},
		{
			name:            "machine with command",
			args:            []string{"machine1", "echo", "hello"},
			wantMachineName: "machine1",
			wantCommand:     []string{"echo", "hello"},
		},
		{
			name:            "machine with separator and command",
			args:            []string{"machine1", "--", "echo", "hello"},
			wantMachineName: "machine1",
			wantCommand:     []string{"echo", "hello"},
		},
		{
			name:            "separator preserves command flag",
			args:            []string{"machine1", "--", "--help"},
			wantMachineName: "machine1",
			wantCommand:     []string{"--help"},
		},
		{
			name:            "only first separator is removed",
			args:            []string{"machine1", "--", "cmd", "--", "arg"},
			wantMachineName: "machine1",
			wantCommand:     []string{"cmd", "--", "arg"},
		},
		{
			name:            "empty args",
			args:            nil,
			wantMachineName: "",
			wantCommand:     []string(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMachineName, gotCommand := normalizeExecArgs(tt.args)
			assert.Equal(t, tt.wantMachineName, gotMachineName)
			assert.Equal(t, tt.wantCommand, gotCommand)
		})
	}
}

func TestValidateExecArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "requires machine",
			args:    nil,
			wantErr: "machine is required",
		},
		{
			name:    "requires command",
			args:    []string{"machine1"},
			wantErr: "command is required",
		},
		{
			name:    "separator without command",
			args:    []string{"machine1", "--"},
			wantErr: "command is required",
		},
		{
			name: "accepts command",
			args: []string{"machine1", "hostname"},
		},
		{
			name: "accepts separator and command flag",
			args: []string{"machine1", "--", "--help"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExecArgs(nil, tt.args)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
