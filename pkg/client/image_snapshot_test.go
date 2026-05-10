package client

import (
	"context"
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListImagesWithSnapshotRequiresMachines(t *testing.T) {
	cli := &Client{}

	_, err := cli.ListImagesWithSnapshot(context.Background(), &ClusterSnapshot{}, api.ImageFilter{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not include machines")
}
