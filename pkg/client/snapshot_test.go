package client

import (
	"context"
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClusterSnapshotDomainNotFound(t *testing.T) {
	fake := &fakeSnapshotClient{getDomainErr: api.ErrNotFound}

	snapshot, err := newClusterSnapshot(context.Background(), fake, ClusterSnapshotOptions{Domain: true})
	require.NoError(t, err)
	assert.Empty(t, snapshot.Domain)
}

func TestClusterSnapshotFindServiceByNameDuplicate(t *testing.T) {
	snapshot := &ClusterSnapshot{
		Services: []api.Service{
			{ID: "svc-1", Name: "web"},
			{ID: "svc-2", Name: "web"},
		},
	}

	_, ok, err := snapshot.FindServiceByName("web")
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "multiple services found with name 'web'")
}

type fakeSnapshotClient struct {
	getDomainErr error
}

func (f *fakeSnapshotClient) ListMachines(context.Context, *api.MachineFilter) (api.MachineMembersList, error) {
	return nil, nil
}

func (f *fakeSnapshotClient) ListServices(context.Context) ([]api.Service, error) {
	return nil, nil
}

func (f *fakeSnapshotClient) GetDomain(context.Context) (string, error) {
	return "", f.getDomainErr
}
