package e2e

import (
	"context"
	"errors"
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func removeServices(t *testing.T, cli api.ServiceClient, names ...string) {
	ctx := context.Background()
	for _, name := range names {
		err := cli.RemoveService(ctx, name, "")
		if !errors.Is(err, api.ErrNotFound) {
			assert.NoError(t, err)
		}
	}
}

func removeVolumes(t *testing.T, cli api.VolumeClient, names ...string) {
	ctx := context.Background()

	volumes, err := cli.ListVolumes(ctx, &api.VolumeFilter{Names: names})
	require.NoError(t, err)

	for _, vol := range volumes {
		err = cli.RemoveVolume(ctx, vol.MachineID, vol.Volume.Name, false)
		assert.NoError(t, err)
	}
}
