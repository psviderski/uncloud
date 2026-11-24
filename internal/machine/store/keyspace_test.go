package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyspace_Subscribe(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ks := newTestKeyspace(t)

	_, _, err := ks.Subscribe(ctx, "", KeyspaceSubscribeOptions{})
	require.Error(t, err)

	subCtx, cancelSub := context.WithCancel(ctx)
	defer cancelSub()
	current, events, err := ks.Subscribe(subCtx, "key", KeyspaceSubscribeOptions{})
	require.NoError(t, err)
	assert.Empty(t, current)

	require.NoError(t, ks.Put(ctx, "key", []byte("v1")))
	assert.Equal(t, recordEvent(t, ks, "key"), receiveEvent(t, events))

	// Other keys, even with the same prefix, don't produce events.
	require.NoError(t, ks.Put(ctx, "key2", []byte("other")))
	require.NoError(t, ks.Put(ctx, "key", []byte("v2")))
	e := receiveEvent(t, events)
	assert.Equal(t, recordEvent(t, ks, "key"), e)
	assert.Equal(t, []byte("v2"), e.Record.Value)

	setInvalidUpdatedAt(t, ks, "key")
	e = receiveEvent(t, events)
	assert.Equal(t, "key", e.Key)
	assert.Nil(t, e.Record)
	assert.ErrorContains(t, e.Err, "parse updated_at")

	require.NoError(t, ks.Put(ctx, "key", []byte("v3")))
	assert.Equal(t, recordEvent(t, ks, "key"), receiveEvent(t, events),
		"subscription must continue after an unreadable record")

	require.NoError(t, ks.Delete(ctx, "key", KeyspaceDeleteOptions{}))
	assert.Equal(t, KeyEvent{Key: "key"}, receiveEvent(t, events))

	// A recreated key is deleted again by its new row.
	require.NoError(t, ks.Put(ctx, "key", []byte("v4")))
	assert.Equal(t, recordEvent(t, ks, "key"), receiveEvent(t, events))
	require.NoError(t, ks.Delete(ctx, "key", KeyspaceDeleteOptions{}))
	assert.Equal(t, KeyEvent{Key: "key"}, receiveEvent(t, events))

	cancelSub()
	select {
	case e, ok := <-events:
		assert.False(t, ok, "channel must be closed without an event after cancelling, got: %+v", e)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "timed out waiting for the channel to close")
	}
}

func TestKeyspace_Subscribe_Prefix(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ks := newTestKeyspace(t)

	require.NoError(t, ks.Put(ctx, "dir/b", []byte("b1")))
	require.NoError(t, ks.Put(ctx, "dir/a", []byte("a1")))
	require.NoError(t, ks.Put(ctx, "dirx", []byte("x1")))
	setInvalidUpdatedAt(t, ks, "dir/b")

	current, events, err := ks.Subscribe(ctx, "dir/", KeyspaceSubscribeOptions{Prefix: true})
	require.NoError(t, err)

	require.Len(t, current, 2)
	assert.Equal(t, recordEvent(t, ks, "dir/a"), current[0])
	assert.Equal(t, "dir/b", current[1].Key)
	assert.Nil(t, current[1].Record)
	assert.ErrorContains(t, current[1].Err, "parse updated_at", "unreadable record must not fail the subscription")

	// Keys outside the prefix don't produce events.
	require.NoError(t, ks.Put(ctx, "dirx", []byte("x2")))
	require.NoError(t, ks.Put(ctx, "dir/c", []byte("c1")))
	assert.Equal(t, recordEvent(t, ks, "dir/c"), receiveEvent(t, events))

	require.NoError(t, ks.Delete(ctx, "dir/a", KeyspaceDeleteOptions{}))
	assert.Equal(t, KeyEvent{Key: "dir/a"}, receiveEvent(t, events))

	// The unreadable dir/b record from the initial records is deleted by its key too.
	require.NoError(t, ks.Delete(ctx, "dir/", KeyspaceDeleteOptions{Prefix: true}))
	deleted := []KeyEvent{receiveEvent(t, events), receiveEvent(t, events)}
	assert.ElementsMatch(t, []KeyEvent{{Key: "dir/b"}, {Key: "dir/c"}}, deleted)
}

func TestKeyspace_Subscribe_WholeKeyspace(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ks := newTestKeyspace(t)
	// The namespace of the other keyspace starts with the namespace of ks.
	other, err := New(ks.corro, nil).Keyspace(ks.namespace + "-other")
	require.NoError(t, err)

	require.NoError(t, ks.Put(ctx, "a", []byte("a1")))
	require.NoError(t, other.Put(ctx, "a", []byte("other")))

	current, events, err := ks.Subscribe(ctx, "", KeyspaceSubscribeOptions{Prefix: true})
	require.NoError(t, err)
	assert.Equal(t, []KeyEvent{recordEvent(t, ks, "a")}, current)

	// Keys of other keyspaces don't produce events.
	require.NoError(t, other.Put(ctx, "b", []byte("other")))
	require.NoError(t, ks.Put(ctx, "b", []byte("b1")))
	assert.Equal(t, recordEvent(t, ks, "b"), receiveEvent(t, events))
}

// recordEvent returns the event that Subscribe should send for the current record at key.
func recordEvent(t *testing.T, ks *Keyspace, key string) KeyEvent {
	t.Helper()

	record, err := ks.Get(t.Context(), key)
	require.NoError(t, err)
	return KeyEvent{Key: key, Record: &record}
}

// setInvalidUpdatedAt makes the record at key unreadable.
func setInvalidUpdatedAt(t *testing.T, ks *Keyspace, key string) {
	t.Helper()

	_, err := ks.corro.ExecContext(t.Context(), "UPDATE cluster SET updated_at = 'invalid' WHERE key = ?",
		ks.namespacedKey(key))
	require.NoError(t, err)
}

func receiveEvent(t *testing.T, events <-chan KeyEvent) KeyEvent {
	t.Helper()

	select {
	case e, ok := <-events:
		require.True(t, ok, "channel closed unexpectedly")
		return e
	case <-time.After(10 * time.Second):
		require.FailNow(t, "timed out waiting for an event")
		return KeyEvent{}
	}
}
