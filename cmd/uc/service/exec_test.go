package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeExecArgs(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantServiceName string
		wantCommand     []string
	}{
		{
			name:            "service only",
			args:            []string{"test-service"},
			wantServiceName: "test-service",
			wantCommand:     []string{},
		},
		{
			name:            "service with command",
			args:            []string{"test-service", "echo", "hello"},
			wantServiceName: "test-service",
			wantCommand:     []string{"echo", "hello"},
		},
		{
			name:            "service with separator and command",
			args:            []string{"test-service", "--", "echo", "hello"},
			wantServiceName: "test-service",
			wantCommand:     []string{"echo", "hello"},
		},
		{
			name:            "service with separator only",
			args:            []string{"test-service", "--"},
			wantServiceName: "test-service",
			wantCommand:     []string{},
		},
		{
			name:            "separator preserves command flag",
			args:            []string{"test-service", "--", "--help"},
			wantServiceName: "test-service",
			wantCommand:     []string{"--help"},
		},
		{
			name:            "only first separator is removed",
			args:            []string{"test-service", "--", "cmd", "--", "arg"},
			wantServiceName: "test-service",
			wantCommand:     []string{"cmd", "--", "arg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotServiceName, gotCommand := normalizeExecArgs(tt.args)
			assert.Equal(t, tt.wantServiceName, gotServiceName)
			assert.Equal(t, tt.wantCommand, gotCommand)
		})
	}
}
