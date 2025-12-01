package client

import (
	"testing"
	"time"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testEntry creates a ServiceLogEntry for testing.
func testEntry(stream api.LogStreamType, ts time.Time, msg string) api.ServiceLogEntry {
	return api.ServiceLogEntry{
		ContainerLogEntry: api.ContainerLogEntry{
			Stream:    stream,
			Timestamp: ts,
			Message:   []byte(msg),
		},
	}
}

// testErrorEntry creates an error ServiceLogEntry for testing.
func testErrorEntry(err error) api.ServiceLogEntry {
	return api.ServiceLogEntry{
		ContainerLogEntry: api.ContainerLogEntry{
			Err: err,
		},
	}
}

func TestLogMerger_EmptyStreams(t *testing.T) {
	t.Parallel()

	merger := NewLogMerger(nil, LogMergerOptions{})
	output := merger.Stream()

	// Output channel should be closed immediately.
	_, ok := <-output
	assert.False(t, ok, "output channel should be closed for empty streams")
}

func TestLogMerger_SingleStream(t *testing.T) {
	t.Parallel()

	ch := make(chan api.ServiceLogEntry, 10)
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch}, LogMergerOptions{})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)

	// Send entries.
	ch <- testEntry(api.LogStreamStdout, t1, "first")
	ch <- testEntry(api.LogStreamStdout, t2, "second")
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

	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2}, LogMergerOptions{})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)
	t3 := t1.Add(2 * time.Second)
	t4 := t1.Add(3 * time.Second)

	// Send interleaved entries from both streams.
	ch1 <- testEntry(api.LogStreamStdout, t1, "ch1-first")
	ch2 <- testEntry(api.LogStreamStdout, t2, "ch2-first")
	ch1 <- testEntry(api.LogStreamStdout, t3, "ch1-second")
	ch2 <- testEntry(api.LogStreamStdout, t4, "ch2-second")

	// Send heartbeats to advance watermark past all entries.
	t5 := t1.Add(4 * time.Second)
	ch1 <- testEntry(api.LogStreamHeartbeat, t5, "")
	ch2 <- testEntry(api.LogStreamHeartbeat, t5, "")

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

	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2}, LogMergerOptions{})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)

	// Stream 1 sends a log entry.
	ch1 <- testEntry(api.LogStreamStdout, t1, "from-ch1")

	// Stream 2 is quiet but sends a heartbeat.
	ch2 <- testEntry(api.LogStreamHeartbeat, t2, "")

	// Now stream 1's entry should be emitted because watermark is t1 (min of t1, t2).
	// We need to advance stream 1's watermark too.
	ch1 <- testEntry(api.LogStreamHeartbeat, t2, "")

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

	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2}, LogMergerOptions{})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)

	// Both streams send entries.
	ch1 <- testEntry(api.LogStreamStdout, t1, "ch1-entry")
	ch2 <- testEntry(api.LogStreamStdout, t2, "ch2-entry")

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
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch}, LogMergerOptions{})
	output := merger.Stream()

	// Send an error entry.
	ch <- testErrorEntry(assert.AnError)

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

	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2}, LogMergerOptions{})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)
	t3 := t1.Add(2 * time.Second)

	// Advance both streams.
	ch1 <- testEntry(api.LogStreamStdout, t2, "ch1-t2")
	ch2 <- testEntry(api.LogStreamStdout, t3, "ch2-t3")

	// Now send a late entry (t1 < watermark which is now t2).
	ch1 <- testEntry(api.LogStreamStdout, t1, "late-entry")

	// Advance watermark to flush buffered entries.
	t4 := t1.Add(4 * time.Second)
	ch1 <- testEntry(api.LogStreamHeartbeat, t4, "")
	ch2 <- testEntry(api.LogStreamHeartbeat, t4, "")

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
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch}, LogMergerOptions{})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)
	t3 := t1.Add(2 * time.Second)
	t4 := t1.Add(3 * time.Second)

	// Send entries out of order within the stream.
	ch <- testEntry(api.LogStreamStdout, t3, "third")
	ch <- testEntry(api.LogStreamStdout, t2, "second")
	ch <- testEntry(api.LogStreamStdout, t4, "forth")
	ch <- testEntry(api.LogStreamStdout, t1, "first")
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
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch}, LogMergerOptions{})
	output := merger.Stream()

	metadata := api.ServiceLogEntryMetadata{
		ServiceID:   "svc-123",
		ServiceName: "my-service",
		MachineID:   "machine-456",
		MachineName: "node-1",
	}

	ch <- api.ServiceLogEntry{
		ContainerLogEntry: api.ContainerLogEntry{
			Timestamp: time.Now(),
			Message:   []byte("test"),
			Stream:    api.LogStreamStdout,
		},
		Metadata: metadata,
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

func TestLogMerger_UnevenStreams(t *testing.T) {
	t.Parallel()

	// Test that entries from streams with different rates are merged correctly in chronological order.
	numFastEntries := maxInFlightPerStream

	// Use buffered channels to allow sends without blocking on receiver.
	ch1 := make(chan api.ServiceLogEntry, numFastEntries+5)
	ch2 := make(chan api.ServiceLogEntry, 10)

	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2}, LogMergerOptions{})
	output := merger.Stream()

	baseTime := time.Now()

	// Stream 1 sends many entries quickly (0ms - 104ms).
	for i := 0; i < numFastEntries; i++ {
		ch1 <- testEntry(api.LogStreamStdout, baseTime.Add(time.Duration(i)*time.Millisecond), "fast")
	}

	// Stream 2 sends entries that interleave with fast entries.
	ch2 <- testEntry(api.LogStreamStdout, baseTime.Add(5*time.Millisecond), "slow")
	ch2 <- testEntry(api.LogStreamStdout, baseTime.Add(50*time.Millisecond), "slow")

	// Send heartbeats to advance watermark past all entries so they get emitted.
	finalTime := baseTime.Add(time.Second)
	ch1 <- testEntry(api.LogStreamHeartbeat, finalTime, "")
	ch2 <- testEntry(api.LogStreamHeartbeat, finalTime, "")

	// Read exactly numFastEntries + 2 log entries.
	expectedCount := numFastEntries + 2
	var results []api.ServiceLogEntry
	for i := 0; i < expectedCount; i++ {
		select {
		case entry := <-output:
			results = append(results, entry)
		case <-time.After(500 * time.Millisecond):
			require.FailNow(t, "timed out waiting for entry %d", i+1)
		}
	}

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

	// Clean up.
	close(ch1)
	close(ch2)
}

func TestLogMerger_StalledStreamExcludedFromWatermark(t *testing.T) {
	t.Parallel()

	ch1 := make(chan api.ServiceLogEntry, 10)
	ch2 := make(chan api.ServiceLogEntry, 10)

	stallTimeout := 100 * time.Millisecond
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2}, LogMergerOptions{
		StallTimeout:       stallTimeout,
		StallCheckInterval: 20 * time.Millisecond,
	})
	output := merger.Stream()

	t0 := time.Now()
	t1 := t0.Add(time.Second)
	t2 := t0.Add(2 * time.Second)

	// Send ch2 entry FIRST - its lastActivity will be oldest.
	ch2 <- testEntry(api.LogStreamStdout, t0, "ch2-entry")

	// Wait so ch2's lastActivity becomes significantly older than ch1's.
	time.Sleep(stallTimeout / 2)

	// Now send ch1 entries - their lastActivity will be newer.
	ch1 <- testEntry(api.LogStreamStdout, t0, "ch1-entry")
	ch1 <- testEntry(api.LogStreamHeartbeat, t1, "")

	// Wait for stall detection. Only ch2 should be stalled because:
	// - ch2's lastActivity is ~stallTimeout old
	// - ch1's lastActivity is only ~stallTimeout/2 old
	// Stall detection emits error first, then updates watermark, then emits ready entries.

	// Drain exactly 3 entries: 1 stall error + 2 log entries.
	var results []api.ServiceLogEntry
	for i := 0; i < 3; i++ {
		select {
		case entry := <-output:
			results = append(results, entry)
		case <-time.After(500 * time.Millisecond):
			require.FailNow(t, "timed out waiting for entry %d", i+1)
		}
	}

	require.Len(t, results, 3)

	// First entry is the stall error (emitted before watermark advances).
	assert.ErrorIs(t, results[0].Err, api.ErrLogStreamStalled)

	// Following entries are the log entries (emitted after watermark advances to t1).
	// Order between entries with the same timestamp is not deterministic.
	logMessages := []string{string(results[1].Message), string(results[2].Message)}
	assert.ElementsMatch(t, []string{"ch1-entry", "ch2-entry"}, logMessages)

	// Now send more entries from stream 1 - they should be emitted without waiting for stream 2.
	ch1 <- testEntry(api.LogStreamStdout, t2, "ch1-later")
	ch1 <- testEntry(api.LogStreamHeartbeat, t2.Add(time.Second), "")

	// Read exactly 1 more log entry.
	select {
	case entry := <-output:
		assert.Equal(t, "ch1-later", string(entry.Message))
	case <-time.After(500 * time.Millisecond):
		require.FailNow(t, "timed out waiting for ch1-later entry")
	}

	// Clean up.
	close(ch1)
	close(ch2)
}
