package operation

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceContainerOperation_StopFirstPreparation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		createErr error
		wantErr   string
		wantCalls []string
	}{
		{
			name: "prepare before cutover",
			wantCalls: []string{
				"create:service",
				"inspect:old",
				"stop:old",
				"start:new",
				"wait:new",
				"remove:old",
			},
		},
		{
			name:      "create failure does not interrupt old container",
			createErr: errors.New("pull failed"),
			wantErr:   "create new container: pull failed",
			wantCalls: []string{"create:service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cli := &recordingClient{
				oldContainerRunning: true,
				createErr:           tt.createErr,
			}
			op := newStopFirstReplaceContainerOperation()

			err := op.Execute(context.Background(), cli)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantCalls, cli.calls)
		})
	}
}

func newStopFirstReplaceContainerOperation() *ReplaceContainerOperation {
	return &ReplaceContainerOperation{
		ServiceID:    "service-id",
		Spec:         api.ServiceSpec{Name: "service"},
		MachineID:    "machine-id",
		MachineName:  "machine",
		OldContainer: api.ServiceContainer{Container: api.Container{InspectResponse: container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{ID: "old"}}}},
		Order:        api.UpdateOrderStopFirst,
	}
}

type recordingClient struct {
	calls               []string
	oldContainerRunning bool
	createErr           error
}

func (c *recordingClient) CreateContainer(
	_ context.Context, _ string, spec api.ServiceSpec, _ string,
) (api.CreateContainerResponse, error) {
	c.calls = append(c.calls, "create:"+spec.Name)
	return api.CreateContainerResponse{
		CreateResponse: container.CreateResponse{ID: "new"},
		Name:           "new",
	}, c.createErr
}

func (c *recordingClient) InspectContainer(
	_ context.Context, _, containerID string,
) (api.MachineServiceContainer, error) {
	c.calls = append(c.calls, "inspect:"+containerID)
	return api.MachineServiceContainer{
		Container: api.ServiceContainer{
			Container: api.Container{InspectResponse: container.InspectResponse{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID:    containerID,
					State: &container.State{Running: c.oldContainerRunning},
				},
			}},
		},
	}, nil
}

func (c *recordingClient) StartContainer(_ context.Context, _, containerID string) error {
	c.calls = append(c.calls, "start:"+containerID)
	return nil
}

func (c *recordingClient) StopContainer(
	_ context.Context, _, containerID string, _ container.StopOptions,
) error {
	c.calls = append(c.calls, "stop:"+containerID)
	return nil
}

func (c *recordingClient) RemoveContainer(
	_ context.Context, _, containerID string, _ container.RemoveOptions,
) error {
	c.calls = append(c.calls, "remove:"+containerID)
	return nil
}

func (c *recordingClient) CreatePreDeployHookContainer(
	context.Context, string, api.ServiceSpec, string,
) (api.CreateContainerResponse, error) {
	panic("unexpected CreatePreDeployHookContainer call")
}

func (c *recordingClient) ExecContainer(context.Context, string, string, api.ExecOptions) (int, error) {
	panic("unexpected ExecContainer call")
}

func (c *recordingClient) WaitContainerHealthy(
	_ context.Context, _, containerID string, _ api.WaitContainerHealthyOptions,
) error {
	c.calls = append(c.calls, "wait:"+containerID)
	return nil
}

func (c *recordingClient) CreateVolume(
	context.Context, string, volume.CreateOptions,
) (api.MachineVolume, error) {
	panic("unexpected CreateVolume call")
}

func (c *recordingClient) ListVolumes(context.Context, *api.VolumeFilter) ([]api.MachineVolume, error) {
	panic("unexpected ListVolumes call")
}

func (c *recordingClient) RemoveVolume(context.Context, string, string, bool) error {
	panic("unexpected RemoveVolume call")
}
