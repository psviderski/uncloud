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
const maxInFlightPerStream = 10

// LogMerger merges multiple log streams into a single chronologically ordered stream based on timestamps.
// It uses a low watermark algorithm to ensure proper ordering across streams.
// Heartbeat entries from streams advance the watermark to enable timely emission of buffered logs.
type LogMerger struct {
	streams   []*mergerStream
	queue     logsHeap
	watermark time.Time
	output    chan api.ServiceLogEntry
}

// NewLogMerger creates a new LogMerger for the given input streams.
func NewLogMerger(streams []<-chan api.ServiceLogEntry) *LogMerger {
	mergerStreams := make([]*mergerStream, len(streams))
	for i, ch := range streams {
		mergerStreams[i] = &mergerStream{
			stream:    ch,
			semaphore: make(chan struct{}, maxInFlightPerStream),
		}
	}

	return &LogMerger{
		streams: mergerStreams,
		output:  make(chan api.ServiceLogEntry),
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
	lastSeen  time.Time // Latest timestamp seen from this stream (log or heartbeat).
	closed    bool      // Whether the stream channel has closed.
}

// streamEvent represents an event from a stream (entry received or stream closed).
type streamEvent struct {
	stream *mergerStream
	entry  api.ServiceLogEntry
	closed bool
}

// run is the main processing loop that merges all streams.
// TODO: check stalled streams, e.g. 30 seconds without heartbeat or log entry?
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

	// Process events and emit entries.
	for e := range events {
		if e.closed {
			e.stream.closed = true

			m.updateWatermark()
			m.emitReadyEntries()
			continue
		}

		// Release semaphore slot now that we've received the entry.
		<-e.stream.semaphore

		// Forward errors immediately.
		if e.entry.Err != nil {
			m.output <- e.entry
			continue
		}

		if e.entry.Timestamp.After(e.stream.lastSeen) {
			e.stream.lastSeen = e.entry.Timestamp
		}
		if e.entry.Stream == api.LogStreamStdout || e.entry.Stream == api.LogStreamStderr {
			heap.Push(&m.queue, e.entry)
		}

		m.updateWatermark()
		m.emitReadyEntries()
	}

	// All streams closed: flush remaining entries in order.
	for m.queue.Len() > 0 {
		entry := heap.Pop(&m.queue).(api.ServiceLogEntry)
		m.output <- entry
	}
}

// updateWatermark recalculates the watermark based on the lastSeen timestamps of all active streams.
func (m *LogMerger) updateWatermark() {
	first := true

	for _, s := range m.streams {
		if s.closed {
			continue // Closed streams don't affect watermark.
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

	for m.queue.Len() > 0 && m.queue[0].Timestamp.Before(m.watermark) {
		entry := heap.Pop(&m.queue).(api.ServiceLogEntry)
		m.output <- entry
	}
}

// logsHeap is a min-heap (heap.Interface) of log entries ordered by timestamp.
type logsHeap []api.ServiceLogEntry

func (h *logsHeap) Len() int {
	return len(*h)
}

func (h *logsHeap) Less(i, j int) bool {
	return (*h)[i].Timestamp.Before((*h)[j].Timestamp)
}

func (h *logsHeap) Swap(i, j int) {
	(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
}

func (h *logsHeap) Push(x any) {
	*h = append(*h, x.(api.ServiceLogEntry))
}

func (h *logsHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
