package client

import (
	"github.com/distribution/reference"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLatestCaddyImage(t *testing.T) {
	t.Parallel()

	image, err := LatestCaddyImage()
	require.NoError(t, err)

	assert.Regexp(t, `^caddy:2\.\d+\.\d+$`, reference.FamiliarString(image))
}
