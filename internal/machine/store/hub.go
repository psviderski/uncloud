package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const (
	hubNamespace     = "hub"
	hubConnectionKey = "connection"
)

// HubConnection is the connection of the cluster to Uncloud Hub. Every machine keeps a tunnel to Hub open while
// the cluster is connected.
type HubConnection struct {
	// Endpoint is the Hub API endpoint that machines open tunnels to.
	Endpoint string
	// ClusterID is the ID of the cluster in Hub.
	ClusterID string
	// CreatedAt is when the cluster was connected. It isn't stored in the record value.
	CreatedAt time.Time `json:"-"`
}

// Hub stores the connection of the cluster to Uncloud Hub in its own keyspace.
type Hub struct {
	ks *Keyspace
}

// GetConnection returns the connection of the cluster to Hub. It returns ErrKeyNotFound if the cluster isn't
// connected.
func (h *Hub) GetConnection(ctx context.Context) (*HubConnection, error) {
	record, err := h.ks.Get(ctx, hubConnectionKey)
	if err != nil {
		return nil, err
	}
	return decodeHubConnection(record)
}

// PutConnection stores the connection of the cluster to Hub, replacing the existing one.
func (h *Hub) PutConnection(ctx context.Context, conn HubConnection) error {
	value, err := json.Marshal(conn)
	if err != nil {
		return fmt.Errorf("marshal Hub connection: %w", err)
	}
	return h.ks.Put(ctx, hubConnectionKey, value)
}

// DeleteConnection deletes the connection of the cluster to Hub if it exists.
func (h *Hub) DeleteConnection(ctx context.Context) error {
	return h.ks.Delete(ctx, hubConnectionKey, KeyspaceDeleteOptions{})
}

// HubConnectionEvent is the state of the connection of the cluster to Hub that [Hub.SubscribeConnection] returns.
type HubConnectionEvent struct {
	// Connection is the connection of the cluster to Hub. It's nil if the cluster isn't connected or Err is set.
	Connection *HubConnection
	// Err is set if the connection can't be read or the subscription failed.
	Err error
}

// SubscribeConnection returns the current connection of the cluster to Hub, and a channel that receives an event
// every time the connection changes. The channel is closed when ctx is cancelled or after an event with
// a subscription error.
func (h *Hub) SubscribeConnection(ctx context.Context) (HubConnectionEvent, <-chan HubConnectionEvent, error) {
	records, events, err := h.ks.Subscribe(ctx, hubConnectionKey, KeyspaceSubscribeOptions{})
	if err != nil {
		return HubConnectionEvent{}, nil, fmt.Errorf("subscribe to Hub connection: %w", err)
	}

	var current HubConnectionEvent
	if len(records) > 0 {
		current = connectionEvent(records[0])
	}

	connEvents := make(chan HubConnectionEvent)
	go func() {
		defer close(connEvents)

		// The events channel is closed when ctx is cancelled.
		for e := range events {
			select {
			case connEvents <- connectionEvent(e):
			case <-ctx.Done():
				return
			}
		}
	}()

	return current, connEvents, nil
}

func connectionEvent(e KeyEvent) HubConnectionEvent {
	if e.Err != nil {
		return HubConnectionEvent{Err: e.Err}
	}
	if e.Record == nil {
		return HubConnectionEvent{}
	}

	conn, err := decodeHubConnection(*e.Record)
	return HubConnectionEvent{Connection: conn, Err: err}
}

func decodeHubConnection(record Record) (*HubConnection, error) {
	var conn HubConnection
	if err := json.Unmarshal(record.Value, &conn); err != nil {
		return nil, fmt.Errorf("unmarshal Hub connection: %w", err)
	}
	conn.CreatedAt = record.UpdatedAt

	return &conn, nil
}
