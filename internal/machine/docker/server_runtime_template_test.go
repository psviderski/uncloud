package docker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	_ "modernc.org/sqlite"
)

func TestCreateServiceContainer_RejectsRuntimeTemplateError(t *testing.T) {
	t.Parallel()

	desiredSpec := api.ServiceSpec{
		Name: "web",
		Container: api.ContainerSpec{
			Image: "busybox:latest",
			VolumeMounts: []api.VolumeMount{
				{VolumeName: "data", ContainerPath: "/data"},
			},
		},
		Volumes: []api.VolumeSpec{
			{
				Name: "data",
				Type: api.VolumeTypeBind,
				BindOptions: &api.BindOptions{
					// Early validation takes the valid branch. Rendering with the real container name takes the invalid one.
					HostPath: `/host/{{if eq .Container.Name "web-a1b2"}}{{.Container.ID}}{{else}}valid{{end}}`,
				},
			},
		},
	}
	specJSON, err := json.Marshal(desiredSpec)
	require.NoError(t, err)

	server := &Server{}
	_, err = server.CreateServiceContainer(context.Background(), &pb.CreateServiceContainerRequest{
		ServiceId:     strings.Repeat("a", 32),
		ServiceSpec:   specJSON,
		ContainerName: "web-a1b2",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.ErrorContains(t, err, "render runtime template in bind volume 'data' host path")
}
