package main

import (
	"context"
	"fmt"
	"github.com/hashicorp/serf/serf"
	crdt "github.com/ipfs/go-ds-crdt"
	"log/slog"
	"time"
)

// Implements the Broadcaster interface.
type SerfBroadcaster struct {
	ctx    context.Context
	serf   *serf.Serf
	nextCh chan []byte
}

// The broadcaster can be shut down by cancelling the given context. This must be done before Closing
// the crdt.Datastore, otherwise things may hang.
func NewSerfBroadcaster(ctx context.Context, serf *serf.Serf) *SerfBroadcaster {
	return &SerfBroadcaster{
		ctx:    ctx,
		serf:   serf,
		nextCh: make(chan []byte),
	}
}

func (b *SerfBroadcaster) Broadcast(bytes []byte) error {
	slog.Debug("Broadcasting head nodes to peers", "size", len(bytes))
	// Other peers are not allowed to coalesce this event by name as the payload may differ.
	// TODO: decode CRDTBroadcast from bytes and embed the nodes content in the payload as well to save round trip.
	if err := b.serf.UserEvent("heads", bytes, false); err != nil {
		return fmt.Errorf("broadcast heads event: %w", err)
	}
	return nil
}

func (b *SerfBroadcaster) Next() ([]byte, error) {
	select {
	case bytes := <-b.nextCh: // Blocks until a new heads event is received.
		return bytes, nil
	case <-b.ctx.Done():
		return nil, crdt.ErrNoMoreBroadcast
	}
}

func (b *SerfBroadcaster) HandleEvent(event serf.Event) {
	select {
	case <-b.ctx.Done():
		return
	default:
	}

	slog.Debug("Received event in broadcaster", "event", event)
	userEvent, ok := event.(serf.UserEvent)
	if !ok {
		// Ignore non-user events.
		return
	}
	if userEvent.Name != "heads" {
		// Ignore non-heads user events.
		return
	}
	start := time.Now()
	select {
	case b.nextCh <- userEvent.Payload:
		slog.Debug("Handled heads event", "duration", time.Since(start))
	case <-b.ctx.Done():
	}
}

var _ crdt.Broadcaster = (*SerfBroadcaster)(nil)
