package logs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseServiceArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    []ServiceArg
		wantErr string
	}{
		{
			name: "empty input",
			args: nil,
			want: []ServiceArg{},
		},
		{
			name: "single bareword service",
			args: []string{"web"},
			want: []ServiceArg{{Service: "web"}},
		},
		{
			name: "single container",
			args: []string{"web/abc123"},
			want: []ServiceArg{{Service: "web", Containers: []string{"abc123"}}},
		},
		{
			name: "multiple containers same service",
			args: []string{"web/abc123", "web/def456"},
			want: []ServiceArg{{Service: "web", Containers: []string{"abc123", "def456"}}},
		},
		{
			name: "multiple services interleaved",
			args: []string{"web/abc123", "api", " web/def456  ", "db/xyz789 "},
			want: []ServiceArg{
				{Service: "web", Containers: []string{"abc123", "def456"}},
				{Service: "api"},
				{Service: "db", Containers: []string{"xyz789"}},
			},
		},
		{
			name: "bareword wins when seen first",
			args: []string{"web", "web/abc123"},
			want: []ServiceArg{{Service: "web"}},
		},
		{
			name: "bareword wins when seen later",
			args: []string{"web/abc123", "web/def456", "web"},
			want: []ServiceArg{{Service: "web"}},
		},
		{
			name: "bareword wins followed by more container args",
			args: []string{"web/abc123", "web", "web/def456"},
			want: []ServiceArg{{Service: "web"}},
		},
		{
			name: "preserves first-seen service order",
			args: []string{"db", "api", "web"},
			want: []ServiceArg{
				{Service: "db"},
				{Service: "api"},
				{Service: "web"},
			},
		},
		{
			name:    "empty arg",
			args:    []string{""},
			wantErr: "empty service argument",
		},
		{
			name:    "empty spaces arg",
			args:    []string{"   "},
			wantErr: "empty service argument",
		},
		{
			name:    "missing service half",
			args:    []string{"/abc123"},
			wantErr: "service name is empty",
		},
		{
			name:    "missing container half",
			args:    []string{"web/"},
			wantErr: "container name or ID is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseServiceArgs(tt.args)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
