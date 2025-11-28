package client

import (
	"container/heap"
	"sync"
	"time"

	"github.com/psviderski/uncloud/pkg/api"
)

// maxInFlightPerStream limits how many entries each input stream can have in the processing queue before being
// throttled. This ensures fair interleaving between streams and prevents one fast stream from causing unbounded
// buffering while waiting for slower streams.
const maxInFlightPerStream = 100

// LogMerger merges multiple log streams into a single chronologically ordered stream based on timestamps.
// It uses a low watermark algorithm to ensure proper ordering across streams.
// Heartbeat entries from streams advance the watermark to enable timely emission of buffered logs.
type LogMerger struct {
	streams      []*mergerStream
	queue        logsHeap
	watermark    time.Time
	output       chan api.ServiceLogEntry
	stallTimeout time.Duration
}

// NewLogMerger creates a new LogMerger for the given input streams. The stallTimeout parameter specifies
// how long a stream can go without receiving any data before it's considered stalled and excluded from
// watermark calculation. A zero timeout disables stall detection.
func NewLogMerger(streams []<-chan api.ServiceLogEntry, stallTimeout time.Duration) *LogMerger {
	mergerStreams := make([]*mergerStream, len(streams))
	now := time.Now()
	for i, ch := range streams {
		mergerStreams[i] = &mergerStream{
			stream:       ch,
			semaphore:    make(chan struct{}, maxInFlightPerStream),
			lastActivity: now,
		}
	}

	return &LogMerger{
		streams:      mergerStreams,
		output:       make(chan api.ServiceLogEntry),
		stallTimeout: stallTimeout,
	}
}

// Stream starts the merge process and returns a channel that emits log entries in chronological order.
// The returned channel is closed when all input streams are closed.
func (m *LogMerger) Stream() <-chan api.ServiceLogEntry {
	if len(m.streams) == 0 {
		close(m.output)
		return m.output
	}

	go m.run()

	return m.output
}

// mergerStream combines a stream channel with its state and flow control.
type mergerStream struct {
	stream    <-chan api.ServiceLogEntry
	semaphore chan struct{}
	// Latest timestamp seen from this stream (log or heartbeat).
	lastSeen time.Time
	// Wall clock time when we last received any data from this stream.
	lastActivity time.Time
	// Metadata associated with this stream. It's populated from the first log entry received.
	metadata *api.ServiceLogEntryMetadata
	// Whether the stream channel has closed.
	closed bool
	// Whether the stream is considered stalled (no data received within timeout).
	stalled bool
}

// streamEvent represents an event from a stream (entry received or stream closed).
type streamEvent struct {
	stream *mergerStream
	entry  api.ServiceLogEntry
	closed bool
}

// queuedEntry wraps a log entry with its source semaphore for release tracking.
type queuedEntry struct {
	entry     api.ServiceLogEntry
	semaphore chan struct{}
}

// run is the main processing loop that merges all streams.
func (m *LogMerger) run() {
	defer close(m.output)

	// Fan-in channel for stream events.
	events := make(chan streamEvent)

	// Start a reader goroutine for each stream to send entries to the events channel with flow control.
	var wg sync.WaitGroup
	for _, stream := range m.streams {
		wg.Go(func() {
			for entry := range stream.stream {
				// Acquire semaphore slot before sending the entry to limit in-flight unprocessed entries per stream.
				stream.semaphore <- struct{}{}
				events <- streamEvent{stream: stream, entry: entry}
			}

			events <- streamEvent{stream: stream, closed: true}
		})
	}

	// Close events channel when all readers finish.
	go func() {
		wg.Wait()
		close(events)
	}()

	// Set up stall detection timer if enabled.
	var stallCh <-chan time.Time
	if m.stallTimeout > 0 {
		stallTicker := time.NewTicker(1 * time.Second)
		stallCh = stallTicker.C
		defer stallTicker.Stop()
	}

	// Process events and emit entries.
	for {
		select {
		case e, ok := <-events:
			if !ok {
				// All streams closed: flush remaining entries in order.
				for m.queue.Len() > 0 {
					qe := heap.Pop(&m.queue).(queuedEntry)
					m.output <- qe.entry
					<-qe.semaphore
				}
				return
			}

			e.stream.lastActivity = time.Now()
			if e.stream.stalled {
				e.stream.stalled = false
			}
			if e.stream.metadata == nil {
				e.stream.metadata = &e.entry.Metadata
			}

			if e.closed {
				e.stream.closed = true

				m.updateWatermark()
				m.emitReadyEntries()
				continue
			}

			// Forward errors immediately and release semaphore.
			if e.entry.Err != nil {
				m.output <- e.entry
				<-e.stream.semaphore
				continue
			}

			if e.entry.Timestamp.After(e.stream.lastSeen) {
				e.stream.lastSeen = e.entry.Timestamp
			}
			if e.entry.Stream == api.LogStreamStdout || e.entry.Stream == api.LogStreamStderr {
				heap.Push(&m.queue, queuedEntry{entry: e.entry, semaphore: e.stream.semaphore})
			} else if e.entry.Stream == api.LogStreamHeartbeat {
				// Release semaphore immediately for the heartbeat entry.
				<-e.stream.semaphore
			}

			m.updateWatermark()
			m.emitReadyEntries()

		case <-stallCh:
			stalled := m.checkStalledStreams()
			if len(stalled) == 0 {
				continue
			}

			for _, s := range stalled {
				errEntry := api.ServiceLogEntry{
					ContainerLogEntry: api.ContainerLogEntry{
						Err: api.ErrLogStreamStalled,
					},
				}
				if s.metadata != nil {
					errEntry.Metadata = *s.metadata
				}

				m.output <- errEntry
			}

			m.updateWatermark()
			m.emitReadyEntries()
		}
	}
}

// checkStalledStreams marks streams as stalled if they haven't received any data within the timeout.
// Returns true if any stream's stalled state changed.
func (m *LogMerger) checkStalledStreams() []*mergerStream {
	var stalled []*mergerStream
	now := time.Now()

	for _, s := range m.streams {
		if s.closed || s.stalled {
			continue
		}

		if now.Sub(s.lastActivity) > m.stallTimeout {
			s.stalled = true
			stalled = append(stalled, s)
		}
	}

	return stalled
}

// updateWatermark recalculates the watermark based on the lastSeen timestamps of all active streams.
func (m *LogMerger) updateWatermark() {
	first := true

	for _, s := range m.streams {
		if s.closed || s.stalled {
			// Closed and stalled streams don't affect watermark.
			continue
		}
		if first || s.lastSeen.Before(m.watermark) {
			m.watermark = s.lastSeen
			first = false
		}
	}
}

// emitReadyEntries pops and emits all buffered entries from the queue with timestamp before the watermark.
func (m *LogMerger) emitReadyEntries() {
	if m.watermark.IsZero() {
		// No entries received yet.
		return
	}

	for m.queue.Len() > 0 && m.queue[0].entry.Timestamp.Before(m.watermark) {
		qe := heap.Pop(&m.queue).(queuedEntry)
		m.output <- qe.entry
		<-qe.semaphore
	}
}

// logsHeap is a min-heap (heap.Interface) of queued entries ordered by timestamp.
type logsHeap []queuedEntry

func (h *logsHeap) Len() int {
	return len(*h)
}

func (h *logsHeap) Less(i, j int) bool {
	return (*h)[i].entry.Timestamp.Before((*h)[j].entry.Timestamp)
}

func (h *logsHeap) Swap(i, j int) {
	(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
}

func (h *logsHeap) Push(x any) {
	*h = append(*h, x.(queuedEntry))
}

func (h *logsHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
