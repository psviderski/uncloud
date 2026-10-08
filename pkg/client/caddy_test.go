package client

import (
	"context"
	"testing"

	"github.com/distribution/reference"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatestCaddyImage(t *testing.T) {
	t.Parallel()

	image, err := latestCaddyImage(context.Background())
	require.NoError(t, err)

	assert.Regexp(t, `^caddy:2\.\d+\.\d+$`, reference.FamiliarString(image))
}

func TestLatestCaddyTag(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		tags []string
		want string
	}{
		{name: "empty", want: "latest"},
		{name: "semantic ordering", tags: []string{"2.9.9", "2.11.4", "2.10.0", "2.11.3"}, want: "2.11.4"},
		{name: "ignore other versions and variants", tags: []string{
			"latest", "1.0.0", "3.0.0", "2.12.0-rc.1", "2.12.0-alpine", "2.12", "v2.12.0", "2.11.4",
		}, want: "2.11.4"},
		{name: "no stable version", tags: []string{"builder", "2.12.0-beta.1"}, want: "latest"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, latestCaddyTag(tt.tags))
		})
	}
}
