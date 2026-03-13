package compose

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterContext(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "no x-context",
			content: `
services:
  web:
    image: nginx
`,
			want: "",
		},
		{
			name: "x-context set",
			content: `
x-context: prod
services:
  web:
    image: nginx
`,
			want: "prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := LoadProjectFromContent(context.Background(), tt.content)
			require.NoError(t, err)
			assert.Equal(t, tt.want, ClusterContext(project))
		})
	}
}
