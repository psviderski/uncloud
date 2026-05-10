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

func TestDeploymentPlanUsesBatchCurrentServices(t *testing.T) {
	project, err := LoadProjectFromContent(context.Background(), composeYAML(20))
	require.NoError(t, err)

	strategy := &recordingStrategy{}
	fake := &composeBatchClient{
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

	assert.Equal(t, 1, fake.batchServiceCalls)
	require.Len(t, fake.batchServiceNamesOrIDs, 1)
	assert.Len(t, fake.batchServiceNamesOrIDs[0], 20)
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

func TestDeploymentPlanErrorsOnDuplicateBatchServiceNames(t *testing.T) {
	project, err := LoadProjectFromContent(context.Background(), composeYAML(1))
	require.NoError(t, err)

	fake := &composeBatchClient{
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

func TestDeploymentPlanLoadsCurrentServicesAtPlanTime(t *testing.T) {
	project, err := LoadProjectFromContent(context.Background(), composeYAML(1))
	require.NoError(t, err)

	strategy := &recordingStrategy{}
	fake := &composeBatchClient{domain: "example.uncld.dev"}

	deployment, err := NewDeploymentWithStrategy(context.Background(), fake, project, strategy)
	require.NoError(t, err)

	fake.services = []api.Service{
		{ID: "svc-01", Name: "svc01", Mode: api.ServiceModeReplicated},
	}
	_, err = deployment.Plan(context.Background())
	require.NoError(t, err)

	require.Len(t, strategy.calls, 1)
	require.NotNil(t, strategy.calls[0].svc)
	assert.Equal(t, "svc-01", strategy.calls[0].svc.ID)
}

func TestDeploymentPlanValidatesBatchCurrentService(t *testing.T) {
	project, err := LoadProjectFromContent(context.Background(), composeYAML(1))
	require.NoError(t, err)

	fake := &composeBatchClient{
		services: []api.Service{
			{ID: "svc-01", Name: "svc01", Mode: api.ServiceModeGlobal},
		},
	}
	deployment, err := NewDeploymentWithStrategy(context.Background(), fake, project, &recordingStrategy{})
	require.NoError(t, err)

	_, err = deployment.Plan(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service mode cannot be changed")
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

type composeBatchClient struct {
	api.Client

	services []api.Service
	domain   string

	batchServiceCalls      int
	batchServiceNamesOrIDs [][]string
	getDomainCalls         int
	inspectServiceCalls    int
}

func (c *composeBatchClient) ListServicesByNameOrID(
	_ context.Context, namesOrIDs []string,
) (map[string]api.Service, error) {
	c.batchServiceCalls++
	c.batchServiceNamesOrIDs = append(c.batchServiceNamesOrIDs, append([]string(nil), namesOrIDs...))

	matched := make(map[string]api.Service, len(namesOrIDs))
	for _, nameOrID := range namesOrIDs {
		if svc, ok := serviceByID(c.services, nameOrID); ok {
			matched[nameOrID] = svc
			continue
		}

		var matches []api.Service
		for _, svc := range c.services {
			if svc.Name == nameOrID {
				matches = append(matches, svc)
			}
		}
		switch len(matches) {
		case 0:
			continue
		case 1:
			matched[nameOrID] = matches[0]
		default:
			return nil, fmt.Errorf("multiple services found with name '%s', use the service ID instead", nameOrID)
		}
	}

	return matched, nil
}

func serviceByID(services []api.Service, id string) (api.Service, bool) {
	for _, svc := range services {
		if svc.ID == id {
			return svc, true
		}
	}
	return api.Service{}, false
}

func (c *composeBatchClient) ListMachines(context.Context, *api.MachineFilter) (api.MachineMembersList, error) {
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

func (c *composeBatchClient) ListVolumes(context.Context, *api.VolumeFilter) ([]api.MachineVolume, error) {
	return nil, nil
}

func (c *composeBatchClient) GetDomain(context.Context) (string, error) {
	c.getDomainCalls++
	return c.domain, nil
}

func (c *composeBatchClient) InspectService(context.Context, string) (api.Service, error) {
	c.inspectServiceCalls++
	return api.Service{}, api.ErrNotFound
}

func (c *composeBatchClient) CreateVolume(
	context.Context, string, volume.CreateOptions,
) (api.MachineVolume, error) {
	return api.MachineVolume{}, nil
}

func (c *composeBatchClient) RemoveVolume(context.Context, string, string, bool) error {
	return nil
}

func (c *composeBatchClient) StopService(context.Context, string, container.StopOptions) error {
	return nil
}
