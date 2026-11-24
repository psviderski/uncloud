package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHub_Connection(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	hub := newTestHub(t)

	_, err := hub.GetConnection(ctx)
	require.ErrorIs(t, err, ErrKeyNotFound)
	require.NoError(t, hub.DeleteConnection(ctx), "deleting a missing connection must succeed")

	require.NoError(t, hub.PutConnection(ctx, HubConnection{Endpoint: "https://hub.test", ClusterID: "c1"}))
	conn, err := hub.GetConnection(ctx)
	require.NoError(t, err)
	assert.Equal(t, "https://hub.test", conn.Endpoint)
	assert.Equal(t, "c1", conn.ClusterID)
	assert.WithinDuration(t, time.Now(), conn.CreatedAt, time.Minute)

	require.NoError(t, hub.DeleteConnection(ctx))
	_, err = hub.GetConnection(ctx)
	require.ErrorIs(t, err, ErrKeyNotFound)
}

func TestHub_SubscribeConnection(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	hub := newTestHub(t)

	subCtx, cancelSub := context.WithCancel(ctx)
	defer cancelSub()
	current, events, err := hub.SubscribeConnection(subCtx)
	require.NoError(t, err)
	assert.Equal(t, HubConnectionEvent{}, current, "cluster must not be connected initially")

	require.NoError(t, hub.PutConnection(ctx, HubConnection{Endpoint: "https://hub.test", ClusterID: "c1"}))
	assert.Equal(t, HubConnectionEvent{Connection: getConnection(t, hub)}, receiveConnectionEvent(t, events))

	require.NoError(t, hub.ks.Put(ctx, hubConnectionKey, []byte("invalid")))
	e := receiveConnectionEvent(t, events)
	assert.Nil(t, e.Connection)
	assert.ErrorContains(t, e.Err, "unmarshal Hub connection")

	setInvalidUpdatedAt(t, hub.ks, hubConnectionKey)
	e = receiveConnectionEvent(t, events)
	assert.Nil(t, e.Connection)
	assert.ErrorContains(t, e.Err, "parse updated_at")

	require.NoError(t, hub.PutConnection(ctx, HubConnection{Endpoint: "https://hub.test", ClusterID: "c2"}))
	assert.Equal(t, HubConnectionEvent{Connection: getConnection(t, hub)}, receiveConnectionEvent(t, events),
		"subscription must continue after an unreadable connection")

	require.NoError(t, hub.DeleteConnection(ctx))
	assert.Equal(t, HubConnectionEvent{}, receiveConnectionEvent(t, events))

	cancelSub()
	select {
	case e, ok := <-events:
		assert.False(t, ok, "channel must be closed without an event after cancelling, got: %+v", e)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "timed out waiting for the channel to close")
	}
}

func TestHub_SubscribeConnection_Existing(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	hub := newTestHub(t)
	require.NoError(t, hub.PutConnection(ctx, HubConnection{Endpoint: "https://hub.test", ClusterID: "c1"}))
	current, _, err := hub.SubscribeConnection(ctx)
	require.NoError(t, err)
	assert.Equal(t, HubConnectionEvent{Connection: getConnection(t, hub)}, current)

	unreadable := newTestHub(t)
	require.NoError(t, unreadable.ks.Put(ctx, hubConnectionKey, []byte("invalid")))
	current, _, err = unreadable.SubscribeConnection(ctx)
	require.NoError(t, err, "unreadable connection must not fail the subscription")
	assert.Nil(t, current.Connection)
	assert.ErrorContains(t, current.Err, "unmarshal Hub connection")
}

// newTestHub returns a Hub store with a unique keyspace in the shared Corrosion.
func newTestHub(t *testing.T) *Hub {
	t.Helper()
	return &Hub{ks: newTestKeyspace(t)}
}

func getConnection(t *testing.T, hub *Hub) *HubConnection {
	t.Helper()

	conn, err := hub.GetConnection(t.Context())
	require.NoError(t, err)
	return conn
}

func receiveConnectionEvent(t *testing.T, events <-chan HubConnectionEvent) HubConnectionEvent {
	t.Helper()

	select {
	case e, ok := <-events:
		require.True(t, ok, "channel closed unexpectedly")
		return e
	case <-time.After(10 * time.Second):
		require.FailNow(t, "timed out waiting for an event")
		return HubConnectionEvent{}
	}
}
