package client

import (
	"testing"
	"time"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogMerger_EmptyStreams(t *testing.T) {
	t.Parallel()

	merger := NewLogMerger(nil)
	output := merger.Stream()

	// Output channel should be closed immediately.
	_, ok := <-output
	assert.False(t, ok, "output channel should be closed for empty streams")
}

func TestLogMerger_SingleStream(t *testing.T) {
	t.Parallel()

	ch := make(chan api.ServiceLogEntry, 10)
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)

	// Send entries.
	ch <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t1, Message: []byte("first")}
	ch <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t2, Message: []byte("second")}
	close(ch)

	// Collect results.
	var results []api.ServiceLogEntry
	for entry := range output {
		results = append(results, entry)
	}

	require.Len(t, results, 2)
	assert.Equal(t, "first", string(results[0].Message))
	assert.Equal(t, "second", string(results[1].Message))
}

func TestLogMerger_BasicMerge(t *testing.T) {
	t.Parallel()

	ch1 := make(chan api.ServiceLogEntry, 10)
	ch2 := make(chan api.ServiceLogEntry, 10)

	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)
	t3 := t1.Add(2 * time.Second)
	t4 := t1.Add(3 * time.Second)

	// Send interleaved entries from both streams.
	ch1 <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t1, Message: []byte("ch1-first")}
	ch2 <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t2, Message: []byte("ch2-first")}
	ch1 <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t3, Message: []byte("ch1-second")}
	ch2 <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t4, Message: []byte("ch2-second")}

	// Send heartbeats to advance watermark past all entries.
	t5 := t1.Add(4 * time.Second)
	ch1 <- api.ServiceLogEntry{Stream: api.LogStreamHeartbeat, Timestamp: t5}
	ch2 <- api.ServiceLogEntry{Stream: api.LogStreamHeartbeat, Timestamp: t5}

	close(ch1)
	close(ch2)

	// Collect results.
	var results []api.ServiceLogEntry
	for entry := range output {
		results = append(results, entry)
	}

	require.Len(t, results, 4)
	assert.Equal(t, "ch1-first", string(results[0].Message))
	assert.Equal(t, "ch2-first", string(results[1].Message))
	assert.Equal(t, "ch1-second", string(results[2].Message))
	assert.Equal(t, "ch2-second", string(results[3].Message))
}

func TestLogMerger_HeartbeatAdvancesWatermark(t *testing.T) {
	t.Parallel()

	ch1 := make(chan api.ServiceLogEntry, 10)
	ch2 := make(chan api.ServiceLogEntry, 10)

	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)

	// Stream 1 sends a log entry.
	ch1 <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t1, Message: []byte("from-ch1")}

	// Stream 2 is quiet but sends a heartbeat.
	ch2 <- api.ServiceLogEntry{Stream: api.LogStreamHeartbeat, Timestamp: t2}

	// Now stream 1's entry should be emitted because watermark is t1 (min of t1, t2).
	// We need to advance stream 1's watermark too.
	ch1 <- api.ServiceLogEntry{Stream: api.LogStreamHeartbeat, Timestamp: t2}

	close(ch1)
	close(ch2)

	var results []api.ServiceLogEntry
	for entry := range output {
		results = append(results, entry)
	}

	require.Len(t, results, 1)
	assert.Equal(t, "from-ch1", string(results[0].Message))
}

func TestLogMerger_StreamClosureFlushesEntries(t *testing.T) {
	t.Parallel()

	ch1 := make(chan api.ServiceLogEntry, 10)
	ch2 := make(chan api.ServiceLogEntry, 10)

	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)

	// Both streams send entries.
	ch1 <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t1, Message: []byte("ch1-entry")}
	ch2 <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t2, Message: []byte("ch2-entry")}

	// Close both streams without heartbeats - entries should still be flushed.
	close(ch1)
	close(ch2)

	var results []api.ServiceLogEntry
	for entry := range output {
		results = append(results, entry)
	}

	require.Len(t, results, 2)
	assert.Equal(t, "ch1-entry", string(results[0].Message))
	assert.Equal(t, "ch2-entry", string(results[1].Message))
}

func TestLogMerger_ErrorForwarding(t *testing.T) {
	t.Parallel()

	ch := make(chan api.ServiceLogEntry, 10)
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch})
	output := merger.Stream()

	// Send an error entry.
	ch <- api.ServiceLogEntry{Err: assert.AnError}

	select {
	case entry := <-output:
		assert.Equal(t, assert.AnError, entry.Err)
	case <-time.After(time.Second):
		require.FailNow(t, "expected error to be emitted out of order")
	}

	close(ch)
}

func TestLogMerger_LateEntryBuffered(t *testing.T) {
	t.Parallel()

	ch1 := make(chan api.ServiceLogEntry, 10)
	ch2 := make(chan api.ServiceLogEntry, 10)

	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)
	t3 := t1.Add(2 * time.Second)

	// Advance both streams.
	ch1 <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t2, Message: []byte("ch1-t2")}
	ch2 <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t3, Message: []byte("ch2-t3")}

	// Now send a late entry (t1 < watermark which is now t2).
	ch1 <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t1, Message: []byte("late-entry")}

	// Advance watermark to flush buffered entries.
	t4 := t1.Add(4 * time.Second)
	ch1 <- api.ServiceLogEntry{Stream: api.LogStreamHeartbeat, Timestamp: t4}
	ch2 <- api.ServiceLogEntry{Stream: api.LogStreamHeartbeat, Timestamp: t4}

	close(ch1)
	close(ch2)

	var results []api.ServiceLogEntry
	for entry := range output {
		results = append(results, entry)
	}

	// All entries should be in results, sorted by timestamp.
	require.Len(t, results, 3)
	// Late entry is sorted to the front due to heap ordering.
	assert.Equal(t, "late-entry", string(results[0].Message))
	assert.Equal(t, "ch1-t2", string(results[1].Message))
	assert.Equal(t, "ch2-t3", string(results[2].Message))
}

func TestLogMerger_OutOfOrderWithinStream(t *testing.T) {
	t.Parallel()

	ch := make(chan api.ServiceLogEntry, 10)
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)
	t3 := t1.Add(2 * time.Second)
	t4 := t1.Add(3 * time.Second)

	// Send entries out of order within the stream.
	ch <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t3, Message: []byte("third")}
	ch <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t2, Message: []byte("second")}
	ch <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t4, Message: []byte("forth")}
	ch <- api.ServiceLogEntry{Stream: api.LogStreamStdout, Timestamp: t1, Message: []byte("first")}
	close(ch)

	var results []api.ServiceLogEntry
	for entry := range output {
		results = append(results, entry)
	}

	require.Len(t, results, 4)
	assert.Equal(t, "second", string(results[0].Message))
	assert.Equal(t, "third", string(results[1].Message))
	assert.Equal(t, "first", string(results[2].Message))
	assert.Equal(t, "forth", string(results[3].Message))
}

func TestLogMerger_PreservesMetadata(t *testing.T) {
	t.Parallel()

	ch := make(chan api.ServiceLogEntry, 10)
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch})
	output := merger.Stream()

	metadata := api.ServiceLogEntryMetadata{
		ServiceID:   "svc-123",
		ServiceName: "my-service",
		MachineID:   "machine-456",
		MachineName: "node-1",
	}

	ch <- api.ServiceLogEntry{
		Timestamp: time.Now(),
		Message:   []byte("test"),
		Stream:    api.LogStreamStdout,
		Metadata:  metadata,
	}
	close(ch)

	var results []api.ServiceLogEntry
	for entry := range output {
		results = append(results, entry)
	}

	require.Len(t, results, 1)
	assert.Equal(t, metadata, results[0].Metadata)
	assert.Equal(t, api.LogStreamStdout, results[0].Stream)
}

func TestLogMerger_FairInterleaving(t *testing.T) {
	t.Parallel()

	// Test that the semaphore limits prevent one fast stream from
	// dominating and causing unbounded buffering.
	ch1 := make(chan api.ServiceLogEntry) // Unbuffered - will block.
	ch2 := make(chan api.ServiceLogEntry) // Unbuffered - will block.

	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2})
	output := merger.Stream()

	baseTime := time.Now()
	numFastEntries := maxInFlightPerStream + 5

	// Stream 1 tries to send many entries quickly.
	// Stream 2 sends entries slowly.
	// With semaphores, stream 1 will be throttled after maxInFlightPerStream entries
	// are queued and will only unblock as entries are emitted.

	ch1Done := make(chan struct{})
	go func() {
		defer close(ch1Done)

		// Send maxInFlightPerStream + 5 entries from stream 1.
		// After maxInFlightPerStream entries are queued, stream 1 will block
		// until entries are emitted (which requires stream 2 to advance watermark).
		for i := 0; i < numFastEntries; i++ {
			ch1 <- api.ServiceLogEntry{
				Stream:    api.LogStreamStdout,
				Timestamp: baseTime.Add(time.Duration(i) * time.Millisecond),
				Message:   []byte("fast"),
			}
		}
		close(ch1)
	}()

	// Concurrently collect results - needed because emitting entries frees semaphore slots.
	resultsCh := make(chan []api.ServiceLogEntry)
	go func() {
		var results []api.ServiceLogEntry
		for entry := range output {
			results = append(results, entry)
		}
		resultsCh <- results
	}()

	// Give stream 1 time to fill its semaphore.
	time.Sleep(50 * time.Millisecond)

	// Stream 2 sends a "slow" entry at 5ms, which should be interleaved with fast entries.
	ch2 <- api.ServiceLogEntry{
		Stream:    api.LogStreamStdout,
		Timestamp: baseTime.Add(5 * time.Millisecond),
		Message:   []byte("slow"),
	}

	// Stream 2 sends another "slow" entry at 50ms to advance the watermark further.
	ch2 <- api.ServiceLogEntry{
		Stream:    api.LogStreamStdout,
		Timestamp: baseTime.Add(50 * time.Millisecond),
		Message:   []byte("slow"),
	}

	// Wait for stream 1 goroutine to finish sending.
	<-ch1Done

	// Close stream 2 to allow merger to finish.
	close(ch2)

	// Collect results.
	results := <-resultsCh

	// We should have all fast entries plus 2 slow entries.
	require.Len(t, results, numFastEntries+2)

	// Count entries from each stream.
	fastCount := 0
	slowCount := 0
	for _, entry := range results {
		switch string(entry.Message) {
		case "fast":
			fastCount++
		case "slow":
			slowCount++
		}
	}
	assert.Equal(t, numFastEntries, fastCount)
	assert.Equal(t, 2, slowCount)

	// Verify entries are in chronological order.
	for i := 1; i < len(results); i++ {
		assert.False(t, results[i].Timestamp.Before(results[i-1].Timestamp),
			"entry %d (ts=%v) should not be before entry %d (ts=%v)",
			i, results[i].Timestamp, i-1, results[i-1].Timestamp)
	}
}
