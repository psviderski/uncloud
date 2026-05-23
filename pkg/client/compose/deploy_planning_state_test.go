package compose

import (
	"context"
	"fmt"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/psviderski/uncloud/pkg/client/deploy/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeploymentPlanUsesPlanningStateForCurrentServices(t *testing.T) {
	project, err := LoadProjectFromContent(context.Background(), composeYAML(20))
	require.NoError(t, err)

	strategy := &recordingStrategy{}
	fake := &composePlanningClient{
		services: []api.Service{
			{ID: "svc-01", Name: "svc01", Mode: api.ServiceModeReplicated},
			{ID: "svc-10", Name: "svc10", Mode: api.ServiceModeReplicated},
		},
		domain: "example.uncld.dev",
	}

	deployment, err := NewDeploymentWithStrategy(context.Background(), fake, project, strategy)
	require.NoError(t, err)
	_, err = deployment.Plan(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, fake.listServicesCalls)
	assert.Equal(t, 1, fake.getDomainCalls)
	assert.Equal(t, 0, fake.inspectServiceCalls)
	require.Len(t, strategy.calls, 20)

	seen := make(map[string]*api.Service)
	for _, call := range strategy.calls {
		seen[call.spec.Name] = call.svc
	}
	require.NotNil(t, seen["svc01"])
	assert.Equal(t, "svc-01", seen["svc01"].ID)
	require.NotNil(t, seen["svc10"])
	assert.Equal(t, "svc-10", seen["svc10"].ID)
	assert.Nil(t, seen["svc02"])
}

func TestDeploymentPlanErrorsOnDuplicateCurrentServiceNames(t *testing.T) {
	project, err := LoadProjectFromContent(context.Background(), composeYAML(1))
	require.NoError(t, err)

	fake := &composePlanningClient{
		services: []api.Service{
			{ID: "svc-a", Name: "svc01"},
			{ID: "svc-b", Name: "svc01"},
		},
	}
	deployment, err := NewDeploymentWithStrategy(context.Background(), fake, project, &recordingStrategy{})
	require.NoError(t, err)

	_, err = deployment.Plan(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple services found with name 'svc01'")
	assert.Equal(t, 0, fake.inspectServiceCalls)
}

func composeYAML(count int) string {
	out := "services:\n"
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("svc%02d", i)
		out += fmt.Sprintf("  %s:\n    image: nginx:%d\n", name, i)
	}
	return out
}

type recordingStrategy struct {
	calls []recordedPlanCall
}

type recordedPlanCall struct {
	svc  *api.Service
	spec api.ServiceSpec
}

func (s *recordingStrategy) Type() string {
	return "recording"
}

func (s *recordingStrategy) Plan(
	_ *scheduler.ClusterState, svc *api.Service, spec api.ServiceSpec,
) (deploy.ServicePlan, error) {
	s.calls = append(s.calls, recordedPlanCall{
		svc:  svc,
		spec: spec,
	})
	serviceID := spec.Name
	if svc != nil {
		serviceID = svc.ID
	}
	return deploy.ServicePlan{
		ServiceID:   serviceID,
		ServiceName: spec.Name,
		Spec:        spec,
	}, nil
}

type composePlanningClient struct {
	api.Client

	services []api.Service
	domain   string

	listServicesCalls   int
	getDomainCalls      int
	inspectServiceCalls int
}

func (c *composePlanningClient) ListServices(context.Context) ([]api.Service, error) {
	c.listServicesCalls++
	return c.services, nil
}

func (c *composePlanningClient) ListMachines(context.Context, *api.MachineFilter) (api.MachineMembersList, error) {
	return api.MachineMembersList{
		{
			Machine: &pb.MachineInfo{
				Id:   "machine-1",
				Name: "machine-1",
			},
			State: pb.MachineMember_UP,
		},
	}, nil
}

func (c *composePlanningClient) ListVolumes(context.Context, *api.VolumeFilter) ([]api.MachineVolume, error) {
	return nil, nil
}

func (c *composePlanningClient) GetDomain(context.Context) (string, error) {
	c.getDomainCalls++
	return c.domain, nil
}

func (c *composePlanningClient) InspectService(context.Context, string) (api.Service, error) {
	c.inspectServiceCalls++
	return api.Service{}, api.ErrNotFound
}

func (c *composePlanningClient) CreateVolume(
	context.Context, string, volume.CreateOptions,
) (api.MachineVolume, error) {
	return api.MachineVolume{}, nil
}

func (c *composePlanningClient) RemoveVolume(context.Context, string, string, bool) error {
	return nil
}

func (c *composePlanningClient) StopService(context.Context, string, container.StopOptions) error {
	return nil
}
