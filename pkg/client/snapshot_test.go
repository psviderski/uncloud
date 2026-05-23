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

func TestNewClusterSnapshotLoadsOnlyRequestedState(t *testing.T) {
	fake := &fakeSnapshotClient{
		services: []api.Service{{ID: "svc-1", Name: "web"}},
		domain:   "example.uncld.dev",
	}

	snapshot, err := newClusterSnapshot(context.Background(), fake, ClusterSnapshotOptions{Services: true})
	require.NoError(t, err)

	assert.Equal(t, fake.services, snapshot.Services)
	assert.Empty(t, snapshot.Domain)
	assert.Equal(t, 1, fake.listServicesCalls)
	assert.Equal(t, 0, fake.getDomainCalls)
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
	services     []api.Service
	domain       string
	getDomainErr error

	listServicesCalls int
	getDomainCalls    int
}

func (f *fakeSnapshotClient) ListServices(context.Context) ([]api.Service, error) {
	f.listServicesCalls++
	return f.services, nil
}

func (f *fakeSnapshotClient) GetDomain(context.Context) (string, error) {
	f.getDomainCalls++
	return f.domain, f.getDomainErr
}
